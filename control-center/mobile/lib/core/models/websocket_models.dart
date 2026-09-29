// WebSocket message models matching the backend contract
// See: control-center/workers/src/types.ts

import 'auth_models.dart';

/// WebSocket connected payload
class RealtimeWebSocketConnectedPayload {
  final String clientId;
  final int userId;
  final String email;
  final UserRole role;
  final List<String> channels;

  RealtimeWebSocketConnectedPayload({
    required this.clientId,
    required this.userId,
    required this.email,
    required this.role,
    required this.channels,
  });

  factory RealtimeWebSocketConnectedPayload.fromJson(
    Map<String, dynamic> json,
  ) {
    return RealtimeWebSocketConnectedPayload(
      clientId: json['client_id'] as String,
      userId: json['user_id'] as int,
      email: json['email'] as String,
      role: UserRole.values.firstWhere(
        (e) => e.name == json['role'],
        orElse: () => UserRole.viewer,
      ),
      channels: (json['channels'] as List).map((e) => e as String).toList(),
    );
  }
}

/// WebSocket subscription payload
class RealtimeWebSocketSubscriptionPayload {
  final String channel;
  final int changed;

  RealtimeWebSocketSubscriptionPayload({
    required this.channel,
    required this.changed,
  });

  factory RealtimeWebSocketSubscriptionPayload.fromJson(
    Map<String, dynamic> json,
  ) {
    return RealtimeWebSocketSubscriptionPayload(
      channel: json['channel'] as String,
      changed: json['changed'] as int,
    );
  }
}

/// WebSocket broadcast payload
class RealtimeWebSocketBroadcastPayload {
  final int fromUserId;
  final String clientId;
  final Map<String, dynamic> data;

  RealtimeWebSocketBroadcastPayload({
    required this.fromUserId,
    required this.clientId,
    required this.data,
  });

  factory RealtimeWebSocketBroadcastPayload.fromJson(
    Map<String, dynamic> json,
  ) {
    final clientId = json['clientId'] as String;
    final fromUserId = json['fromUserId'] as int;
    final data = Map<String, dynamic>.from(json)
      ..remove('clientId')
      ..remove('fromUserId');
    return RealtimeWebSocketBroadcastPayload(
      fromUserId: fromUserId,
      clientId: clientId,
      data: data,
    );
  }
}

/// WebSocket message types
enum WebSocketMessageType {
  connected,
  ping,
  pong,
  error,
  subscribe,
  unsubscribe,
  subscribed,
  unsubscribed,
  message,
  broadcast,
}

/// Base WebSocket message
abstract class RealtimeWebSocketMessage {
  WebSocketMessageType get type;
}

/// Connected message
class RealtimeWebSocketConnectedMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.connected;

  final String id;
  final String timestamp;
  final String version;
  final RealtimeWebSocketConnectedPayload payload;

  RealtimeWebSocketConnectedMessage({
    required this.id,
    required this.timestamp,
    required this.version,
    required this.payload,
  });

  factory RealtimeWebSocketConnectedMessage.fromJson(
    Map<String, dynamic> json,
  ) {
    return RealtimeWebSocketConnectedMessage(
      id: json['id'] as String,
      timestamp: json['timestamp'] as String,
      version: json['version'] as String,
      payload: RealtimeWebSocketConnectedPayload.fromJson(
        json['payload'] as Map<String, dynamic>,
      ),
    );
  }
}

/// Ping message
class RealtimeWebSocketPingMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.ping;

  RealtimeWebSocketPingMessage();
}

/// Pong message
class RealtimeWebSocketPongMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.pong;

  RealtimeWebSocketPongMessage();
}

/// Error message
class RealtimeWebSocketErrorMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.error;

  final String payload;

  RealtimeWebSocketErrorMessage({required this.payload});

  factory RealtimeWebSocketErrorMessage.fromJson(Map<String, dynamic> json) {
    return RealtimeWebSocketErrorMessage(
      payload: json['payload'] as String,
    );
  }
}

/// Subscribe message (outbound)
class RealtimeWebSocketSubscribeMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.subscribe;

  final String payload;

  RealtimeWebSocketSubscribeMessage({required this.payload});

  Map<String, dynamic> toJson() => {
        'type': 'subscribe',
        'payload': payload,
      };
}

/// Unsubscribe message (outbound)
class RealtimeWebSocketUnsubscribeMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.unsubscribe;

  final String payload;

  RealtimeWebSocketUnsubscribeMessage({required this.payload});

  Map<String, dynamic> toJson() => {
        'type': 'unsubscribe',
        'payload': payload,
      };
}

/// Subscribed message (inbound)
class RealtimeWebSocketSubscribedMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.subscribed;

  final RealtimeWebSocketSubscriptionPayload payload;

  RealtimeWebSocketSubscribedMessage({required this.payload});

  factory RealtimeWebSocketSubscribedMessage.fromJson(
    Map<String, dynamic> json,
  ) {
    return RealtimeWebSocketSubscribedMessage(
      payload: RealtimeWebSocketSubscriptionPayload.fromJson(
        json['payload'] as Map<String, dynamic>,
      ),
    );
  }
}

/// Unsubscribed message (inbound)
class RealtimeWebSocketUnsubscribedMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.unsubscribed;

  final RealtimeWebSocketSubscriptionPayload payload;

  RealtimeWebSocketUnsubscribedMessage({required this.payload});

  factory RealtimeWebSocketUnsubscribedMessage.fromJson(
    Map<String, dynamic> json,
  ) {
    return RealtimeWebSocketUnsubscribedMessage(
      payload: RealtimeWebSocketSubscriptionPayload.fromJson(
        json['payload'] as Map<String, dynamic>,
      ),
    );
  }
}

/// Broadcast message (inbound)
class RealtimeWebSocketBroadcastMessage extends RealtimeWebSocketMessage {
  @override
  WebSocketMessageType get type => WebSocketMessageType.message;

  final RealtimeWebSocketBroadcastPayload payload;
  final String timestamp;

  RealtimeWebSocketBroadcastMessage({
    required this.payload,
    required this.timestamp,
  });

  factory RealtimeWebSocketBroadcastMessage.fromJson(
    Map<String, dynamic> json,
  ) {
    return RealtimeWebSocketBroadcastMessage(
      payload: RealtimeWebSocketBroadcastPayload.fromJson(
        json['payload'] as Map<String, dynamic>,
      ),
      timestamp: json['timestamp'] as String,
    );
  }
}

/// Parse a WebSocket message from JSON
RealtimeWebSocketMessage? parseWebSocketMessage(Map<String, dynamic> json) {
  final type = json['type'] as String?;
  switch (type) {
    case 'connected':
      return RealtimeWebSocketConnectedMessage.fromJson(json);
    case 'ping':
      return RealtimeWebSocketPingMessage();
    case 'pong':
      return RealtimeWebSocketPongMessage();
    case 'error':
      return RealtimeWebSocketErrorMessage.fromJson(json);
    case 'subscribed':
      return RealtimeWebSocketSubscribedMessage.fromJson(json);
    case 'unsubscribed':
      return RealtimeWebSocketUnsubscribedMessage.fromJson(json);
    case 'message':
      return RealtimeWebSocketBroadcastMessage.fromJson(json);
    default:
      return null;
  }
}