import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;

/// SSE Service for real-time communication with Workers API
/// Uses Server-Sent Events instead of WebSocket due to Cloudflare Durable Object issues
class SSEService {
  SSEService({http.Client Function()? clientFactory})
      : _clientFactory = clientFactory ?? http.Client.new;

  final http.Client Function() _clientFactory;
  http.Client? _streamClient;
  final Map<String, List<Function(dynamic)>> _handlers = {};
  Timer? _reconnectTimer;
  String? _url;
  String? _token;
  bool _isConnecting = false;
  bool _isConnected = false;
  bool _shouldReconnect = true;
  int _reconnectAttempts = 0;
  static const int _maxReconnectAttempts = 10;
  static const Duration _baseReconnectDelay = Duration(seconds: 1);
  static const Duration _maxReconnectDelay = Duration(seconds: 30);

  /// Whether the SSE is connected
  bool get isConnected => _isConnected;

  /// Stream of connection state changes
  final StreamController<bool> _connectionStateController =
      StreamController<bool>.broadcast();
  Stream<bool> get connectionState => _connectionStateController.stream;

  /// Current subscribed channels
  final Set<String> _subscribedChannels = {};
  Set<String> get subscribedChannels => Set.unmodifiable(_subscribedChannels);

  /// Connect to SSE endpoint
  Future<void> connect(String url, {String? token}) async {
    if (_isConnecting) return;
    if (_isConnected && _url == url && _token == token) return;

    _reconnectTimer?.cancel();
    _streamClient?.close();
    _streamClient = null;

    _url = url;
    _token = token;
    _isConnecting = true;
    _shouldReconnect = true;

    try {
      final client = _clientFactory();
      _streamClient = client;

      final request = http.Request('GET', Uri.parse(url));
      request.headers['Accept'] = 'text/event-stream';
      request.headers['Cache-Control'] = 'no-cache';
      if (token != null && token.isNotEmpty) {
        request.headers['Authorization'] = 'Bearer $token';
      }

      final response = await client.send(request);

      if (response.statusCode != 200) {
        throw Exception('SSE connection failed: ${response.statusCode}');
      }

      _isConnecting = false;
      _isConnected = true;
      _reconnectAttempts = 0;
      _connectionStateController.add(true);

      final stream = response.stream.transform(utf8.decoder);
      var buffer = '';

      await for (final chunk in stream) {
        if (!_shouldReconnect) break;

        buffer += chunk;
        _processBuffer(buffer);
        buffer = _clearProcessedEvents(buffer);
      }

      if (_shouldReconnect) {
        _handleDisconnect(scheduleReconnect: true);
      }
    } catch (_) {
      _handleDisconnect(scheduleReconnect: _shouldReconnect);
    } finally {
      _isConnecting = false;
    }
  }

  /// Process the buffer for complete events
  void _processBuffer(String buffer) {
    final events = buffer.split('\n\n');

    for (var i = 0; i < events.length - 1; i++) {
      final event = events[i].trim();
      if (event.isNotEmpty) {
        _parseAndDispatchEvent(event);
      }
    }
  }

  /// Clear processed events from buffer
  String _clearProcessedEvents(String buffer) {
    final lastSeparator = buffer.lastIndexOf('\n\n');
    if (lastSeparator >= 0) {
      return buffer.substring(lastSeparator + 2);
    }
    return buffer;
  }

  /// Parse and dispatch an SSE event
  void _parseAndDispatchEvent(String eventText) {
    String? eventType;
    String? data;

    for (final line in eventText.split('\n')) {
      if (line.startsWith('event:')) {
        eventType = line.substring(6).trim();
      } else if (line.startsWith('data:')) {
        data = line.substring(5).trim();
      } else if (line.startsWith(':')) {
        // Comment line (heartbeat), ignore
      }
    }

    if (eventText.contains(': heartbeat')) {
      return;
    }

    dynamic parsedData;
    if (data != null && data.isNotEmpty) {
      try {
        parsedData = jsonDecode(data);
      } catch (_) {
        parsedData = data;
      }
    }

    if (eventType != null && _handlers.containsKey(eventType)) {
      for (final handler in List<Function(dynamic)>.from(_handlers[eventType]!)) {
        try {
          handler(parsedData);
        } catch (_) {}
      }
    }

    if (parsedData is Map && parsedData.containsKey('type')) {
      final type = parsedData['type'] as String;
      if (_handlers.containsKey(type)) {
        for (final handler in List<Function(dynamic)>.from(_handlers[type]!)) {
          try {
            handler(parsedData['payload'] ?? parsedData);
          } catch (_) {}
        }
      }
    }

    if (_handlers.containsKey('message')) {
      for (final handler in List<Function(dynamic)>.from(_handlers['message']!)) {
        try {
          handler(parsedData);
        } catch (_) {}
      }
    }
  }

  Uri _buildChannelUri(String action) {
    final sseUri = Uri.parse(_url!);
    final basePath = sseUri.path.endsWith('/sse')
        ? sseUri.path.substring(0, sseUri.path.length - 4)
        : sseUri.path;
    return sseUri.replace(path: '$basePath/sse/$action', query: null);
  }

  /// Subscribe to a channel via REST API
  Future<bool> subscribe(String channel) async {
    if (_url == null || _token == null) return false;

    final client = _clientFactory();
    try {
      final response = await client.post(
        _buildChannelUri('subscribe'),
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer $_token',
        },
        body: jsonEncode({'channel': channel}),
      );

      if (response.statusCode == 200) {
        _subscribedChannels.add(channel);
        return true;
      }
    } catch (_) {
      // Subscribe failed
    } finally {
      client.close();
    }
    return false;
  }

  /// Unsubscribe from a channel
  Future<bool> unsubscribe(String channel) async {
    if (_url == null || _token == null) return false;

    final client = _clientFactory();
    try {
      final response = await client.post(
        _buildChannelUri('unsubscribe'),
        headers: {
          'Content-Type': 'application/json',
          'Authorization': 'Bearer $_token',
        },
        body: jsonEncode({'channel': channel}),
      );

      if (response.statusCode == 200) {
        _subscribedChannels.remove(channel);
        return true;
      }
    } catch (_) {
      // Unsubscribe failed
    } finally {
      client.close();
    }
    return false;
  }

  @visibleForTesting
  void parseTestEvent(String rawEvent) {
    _parseAndDispatchEvent(rawEvent);
  }

  /// Register a handler for an event type
  void on(String eventType, Function(dynamic) handler) {
    final handlers = _handlers.putIfAbsent(eventType, () => []);
    if (!handlers.contains(handler)) {
      handlers.add(handler);
    }
  }

  /// Remove a handler for an event type
  void off(String eventType, [Function(dynamic)? handler]) {
    if (handler == null) {
      _handlers.remove(eventType);
      return;
    }

    final handlers = _handlers[eventType];
    handlers?.remove(handler);
    if (handlers != null && handlers.isEmpty) {
      _handlers.remove(eventType);
    }
  }

  void _handleDisconnect({required bool scheduleReconnect}) {
    _streamClient?.close();
    _streamClient = null;

    final wasConnected = _isConnected;
    _isConnected = false;
    _isConnecting = false;

    if (wasConnected || scheduleReconnect) {
      _connectionStateController.add(false);
    }

    if (scheduleReconnect && _shouldReconnect) {
      _scheduleReconnect();
    }
  }

  /// Disconnect from SSE endpoint
  void disconnect() {
    _shouldReconnect = false;
    _reconnectTimer?.cancel();
    _streamClient?.close();
    _streamClient = null;
    _isConnected = false;
    _isConnecting = false;
    _subscribedChannels.clear();
    _connectionStateController.add(false);
  }

  void _scheduleReconnect() {
    if (!_shouldReconnect) return;
    if (_reconnectAttempts >= _maxReconnectAttempts) return;

    _reconnectTimer?.cancel();

    final delay = Duration(
      milliseconds: (_baseReconnectDelay.inMilliseconds *
              (1 << _reconnectAttempts.clamp(0, 5)))
          .clamp(
            _baseReconnectDelay.inMilliseconds,
            _maxReconnectDelay.inMilliseconds,
          ),
    );

    _reconnectTimer = Timer(delay, () {
      _reconnectAttempts++;
      if (_url != null) {
        unawaited(connect(_url!, token: _token));
      }
    });
  }

  /// Reset reconnection attempts (call on successful operation)
  void resetReconnectAttempts() {
    _reconnectAttempts = 0;
  }

  /// Dispose resources
  void dispose() {
    disconnect();
    _handlers.clear();
    _connectionStateController.close();
  }
}

/// Global SSE service instance
final SSEService sseService = SSEService();
