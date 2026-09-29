// Token and Session models for user authentication
class ApiToken {
  final String id;
  final String name;
  final String? token;
  final DateTime createdAt;
  final DateTime? lastUsedAt;
  final DateTime? expiresAt;

  const ApiToken({
    required this.id,
    required this.name,
    this.token,
    required this.createdAt,
    this.lastUsedAt,
    this.expiresAt,
  });

  factory ApiToken.fromJson(Map<String, dynamic> json) {
    return ApiToken(
      id: json['id']?.toString() ?? '',
      name: json['name'] ?? '',
      token: json['token'],
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at']) ?? DateTime.now()
          : DateTime.now(),
      lastUsedAt: json['last_used_at'] != null
          ? DateTime.tryParse(json['last_used_at'])
          : null,
      expiresAt: json['expires_at'] != null
          ? DateTime.tryParse(json['expires_at'])
          : null,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'name': name,
      'token': token,
      'created_at': createdAt.toIso8601String(),
      'last_used_at': lastUsedAt?.toIso8601String(),
      'expires_at': expiresAt?.toIso8601String(),
    };
  }

  bool get isExpired {
    if (expiresAt == null) return false;
    return DateTime.now().isAfter(expiresAt!);
  }
}

/// Session model
class Session {
  final String id;
  final String device;
  final String? ipAddress;
  final String? location;
  final DateTime createdAt;
  final DateTime lastActiveAt;
  final bool isCurrent;

  const Session({
    required this.id,
    required this.device,
    this.ipAddress,
    this.location,
    required this.createdAt,
    required this.lastActiveAt,
    this.isCurrent = false,
  });

  factory Session.fromJson(Map<String, dynamic> json) {
    return Session(
      id: json['id']?.toString() ?? '',
      device: json['device'] ?? json['user_agent'] ?? 'Unknown Device',
      ipAddress: json['ip_address'],
      location: json['location'],
      createdAt: json['created_at'] != null
          ? DateTime.tryParse(json['created_at']) ?? DateTime.now()
          : DateTime.now(),
      lastActiveAt: json['last_active_at'] != null
          ? DateTime.tryParse(json['last_active_at']) ?? DateTime.now()
          : DateTime.now(),
      isCurrent: json['is_current'] ?? false,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'device': device,
      'ip_address': ipAddress,
      'location': location,
      'created_at': createdAt.toIso8601String(),
      'last_active_at': lastActiveAt.toIso8601String(),
      'is_current': isCurrent,
    };
  }
}