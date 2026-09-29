import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/services/api_client.dart';
import 'package:anixops_mobile/core/services/websocket_service.dart';
import 'package:anixops_mobile/core/services/sse_service.dart';

/// Provider for the API client
final apiClientProvider = Provider<ApiClient>((ref) {
  return apiClient;
});

/// Provider for the WebSocket service
final webSocketServiceProvider = Provider<WebSocketService>((ref) {
  return webSocketService;
});

/// Provider for WebSocket connection state
final wsConnectionStateProvider = StreamProvider<bool>((ref) {
  final ws = ref.watch(webSocketServiceProvider);
  return ws.connectionState;
});

/// Provider for the SSE service
final sseServiceProvider = Provider<SSEService>((ref) {
  return sseService;
});

/// Provider for SSE connection state
final sseConnectionStateProvider = StreamProvider<bool>((ref) {
  final sse = ref.watch(sseServiceProvider);
  return sse.connectionState;
});

/// SSE configuration
class SSEConfig {
  static const String defaultUrl = 'https://api.anixops.com/api/v1/sse';
  static const List<String> defaultChannels = ['global', 'nodes', 'tasks', 'logs'];
}