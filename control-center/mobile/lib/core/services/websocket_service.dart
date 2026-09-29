import 'dart:async';
import 'dart:convert';
import 'package:web_socket_channel/web_socket_channel.dart';
import 'package:web_socket_channel/status.dart' as status;

/// WebSocket service for real-time communication with Workers API
class WebSocketService {
  WebSocketChannel? _channel;
  final Map<String, List<Function(dynamic)>> _handlers = {};
  Timer? _reconnectTimer;
  Timer? _heartbeatTimer;
  String? _url;
  String? _token;
  bool _isConnecting = false;
  bool _shouldReconnect = true;
  int _reconnectAttempts = 0;
  static const int _maxReconnectAttempts = 10;
  static const Duration _baseReconnectDelay = Duration(seconds: 1);
  static const Duration _maxReconnectDelay = Duration(seconds: 30);
  static const Duration _heartbeatInterval = Duration(seconds: 30);

  /// Whether the WebSocket is connected
  bool get isConnected => _channel != null && !_isConnecting;

  /// Stream of connection state changes
  final StreamController<bool> _connectionStateController = StreamController<bool>.broadcast();
  Stream<bool> get connectionState => _connectionStateController.stream;

  /// Subscribed channels
  final Set<String> _subscribedChannels = {};
  Set<String> get subscribedChannels => Set.unmodifiable(_subscribedChannels);

  /// Connect to WebSocket server
  /// Note: Workers API uses Bearer token in Authorization header
  /// For WebSocket, we pass token in query parameter
  Future<void> connect(String url, {String? token}) async {
    if (_isConnecting || isConnected) return;

    _url = url;
    _token = token;
    _isConnecting = true;
    _shouldReconnect = true;

    try {
      final uri = Uri.parse(url);

      // Convert HTTP(S) to WS(S)
      String wsScheme;
      if (uri.scheme == 'https' || uri.scheme == 'wss') {
        wsScheme = 'wss';
      } else {
        wsScheme = 'ws';
      }

      // Build WebSocket URL with token for auth
      // Workers API expects token in query param or header
      final wsUri = Uri(
        scheme: wsScheme,
        host: uri.host,
        port: uri.port,
        path: uri.path,
        queryParameters: token != null ? {'token': token} : null,
      );

      _channel = WebSocketChannel.connect(wsUri);

      await _channel!.ready;
      _isConnecting = false;
      _reconnectAttempts = 0;
      _connectionStateController.add(true);

      // Start heartbeat
      _startHeartbeat();

      // Listen for messages
      _channel!.stream.listen(
        _onMessage,
        onError: _onError,
        onDone: _onDone,
      );
    } catch (e) {
      _isConnecting = false;
      _connectionStateController.add(false);
      if (_shouldReconnect) {
        _scheduleReconnect();
      }
    }
  }

  /// Disconnect from WebSocket server
  void disconnect() {
    _shouldReconnect = false;
    _reconnectTimer?.cancel();
    _heartbeatTimer?.cancel();
    _channel?.sink.close(status.goingAway);
    _channel = null;
    _subscribedChannels.clear();
    _connectionStateController.add(false);
  }

  /// Subscribe to an event type
  void on(String eventType, Function(dynamic) handler) {
    _handlers.putIfAbsent(eventType, () => []).add(handler);
  }

  /// Unsubscribe from an event type
  void off(String eventType, [Function(dynamic)? handler]) {
    if (handler == null) {
      _handlers.remove(eventType);
    } else {
      _handlers[eventType]?.remove(handler);
    }
  }

  /// Send a message to the server
  void send(String type, {dynamic payload}) {
    if (!isConnected) return;

    final message = jsonEncode({
      'type': type,
      'payload': payload,
    });

    _channel?.sink.add(message);
  }

  /// Subscribe to a channel
  void subscribeToChannel(String channel) {
    _subscribedChannels.add(channel);
    send('subscribe', payload: channel);
  }

  /// Unsubscribe from a channel
  void unsubscribeFromChannel(String channel) {
    _subscribedChannels.remove(channel);
    send('unsubscribe', payload: channel);
  }

  void _onMessage(dynamic message) {
    try {
      final decoded = jsonDecode(message);

      // Workers API format: { type: '...', payload: ..., timestamp: '...' }
      final type = decoded['type'] as String?;
      final payload = decoded['payload'] ?? decoded['data'];

      // Handle pong
      if (type == 'pong') {
        return;
      }

      // Dispatch to handlers by type
      if (type != null && _handlers.containsKey(type)) {
        for (final handler in List.from(_handlers[type]!)) {
          try {
            handler(payload);
          } catch (e) {
            // Handler error, continue
          }
        }
      }

      // Also dispatch to 'message' handlers
      if (_handlers.containsKey('message')) {
        for (final handler in List.from(_handlers['message']!)) {
          try {
            handler(decoded);
          } catch (e) {
            // Handler error, continue
          }
        }
      }
    } catch (e) {
      // Handle parse error
    }
  }

  void _onError(dynamic error) {
    _connectionStateController.add(false);
    if (_shouldReconnect) {
      _scheduleReconnect();
    }
  }

  void _onDone() {
    _connectionStateController.add(false);
    if (_shouldReconnect) {
      _scheduleReconnect();
    }
  }

  void _scheduleReconnect() {
    if (!_shouldReconnect) return;
    if (_reconnectAttempts >= _maxReconnectAttempts) return;

    _reconnectTimer?.cancel();

    // Exponential backoff
    final delayMs = (_baseReconnectDelay.inMilliseconds *
            (1 << _reconnectAttempts.clamp(0, 5)))
        .clamp(_baseReconnectDelay.inMilliseconds, _maxReconnectDelay.inMilliseconds);

    _reconnectTimer = Timer(Duration(milliseconds: delayMs), () {
      _reconnectAttempts++;
      if (_url != null) {
        connect(_url!, token: _token);
      }
    });
  }

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(_heartbeatInterval, (_) {
      send('ping');
    });
  }

  /// Reset reconnection attempts
  void resetReconnectAttempts() {
    _reconnectAttempts = 0;
  }

  /// Dispose resources
  void dispose() {
    disconnect();
    _connectionStateController.close();
  }
}

/// Global WebSocket service instance
final WebSocketService webSocketService = WebSocketService();

/// WebSocket event types from Workers API
class WebSocketEventTypes {
  static const String connected = 'connected';
  static const String ping = 'ping';
  static const String pong = 'pong';
  static const String subscribed = 'subscribed';
  static const String unsubscribed = 'unsubscribed';
  static const String nodeUpdate = 'node_update';
  static const String taskUpdate = 'task_update';
  static const String log = 'log';
  static const String message = 'message';
  static const String error = 'error';
}