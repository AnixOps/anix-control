// Node models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Node status type
enum NodeStatus { online, offline, maintenance }

/// Node entity
class Node {
  final int id;
  final String name;
  final String host;
  final int port;
  final NodeStatus status;
  final String? lastSeen;
  final String? config;
  final String? agentId;
  final String? agentSecret;
  final String? agentVersion;
  final String? os;
  final String? arch;
  final int? cpuCount;
  final double? memoryGb;
  final double? diskGb;
  final String createdAt;
  final String updatedAt;

  Node({
    required this.id,
    required this.name,
    required this.host,
    required this.port,
    required this.status,
    this.lastSeen,
    this.config,
    this.agentId,
    this.agentSecret,
    this.agentVersion,
    this.os,
    this.arch,
    this.cpuCount,
    this.memoryGb,
    this.diskGb,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Node.fromJson(Map<String, dynamic> json) {
    return Node(
      id: json['id'] as int,
      name: json['name'] as String,
      host: json['host'] as String,
      port: json['port'] as int,
      status: NodeStatus.values.firstWhere(
        (e) => e.name == json['status'],
        orElse: () => NodeStatus.offline,
      ),
      lastSeen: json['last_seen'] as String?,
      config: json['config'] as String?,
      agentId: json['agent_id'] as String?,
      agentSecret: json['agent_secret'] as String?,
      agentVersion: json['agent_version'] as String?,
      os: json['os'] as String?,
      arch: json['arch'] as String?,
      cpuCount: json['cpu_count'] as int?,
      memoryGb: (json['memory_gb'] as num?)?.toDouble(),
      diskGb: (json['disk_gb'] as num?)?.toDouble(),
      createdAt: json['created_at'] as String,
      updatedAt: json['updated_at'] as String,
    );
  }
}

/// Node list response data
class NodeSummaryResponseData {
  final List<Node> items;
  final int total;
  final int page;
  final int perPage;
  final int totalPages;

  NodeSummaryResponseData({
    required this.items,
    required this.total,
    required this.page,
    required this.perPage,
    required this.totalPages,
  });

  factory NodeSummaryResponseData.fromJson(Map<String, dynamic> json) {
    return NodeSummaryResponseData(
      items: (json['items'] as List)
          .map((e) => Node.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
      page: json['page'] as int,
      perPage: json['per_page'] as int,
      totalPages: json['total_pages'] as int,
    );
  }
}

/// Node action response data
class NodeActionResponseData {
  final String id;
  final String status;

  NodeActionResponseData({
    required this.id,
    required this.status,
  });

  factory NodeActionResponseData.fromJson(Map<String, dynamic> json) {
    return NodeActionResponseData(
      id: json['id'] as String,
      status: json['status'] as String,
    );
  }
}

/// Network stats
class NodeNetworkStats {
  final double upload;
  final double download;

  NodeNetworkStats({required this.upload, required this.download});

  factory NodeNetworkStats.fromJson(Map<String, dynamic> json) {
    return NodeNetworkStats(
      upload: (json['upload'] as num).toDouble(),
      download: (json['download'] as num).toDouble(),
    );
  }
}

/// Node stats response data
class NodeStatsResponseData {
  final int nodeId;
  final NodeStatus status;
  final int uptime;
  final double cpuUsage;
  final double memoryUsage;
  final double diskUsage;
  final NodeNetworkStats network;
  final int connections;
  final int users;
  final String lastUpdated;

  NodeStatsResponseData({
    required this.nodeId,
    required this.status,
    required this.uptime,
    required this.cpuUsage,
    required this.memoryUsage,
    required this.diskUsage,
    required this.network,
    required this.connections,
    required this.users,
    required this.lastUpdated,
  });

  factory NodeStatsResponseData.fromJson(Map<String, dynamic> json) {
    return NodeStatsResponseData(
      nodeId: json['node_id'] as int,
      status: NodeStatus.values.firstWhere(
        (e) => e.name == json['status'],
        orElse: () => NodeStatus.offline,
      ),
      uptime: json['uptime'] as int,
      cpuUsage: (json['cpu_usage'] as num).toDouble(),
      memoryUsage: (json['memory_usage'] as num).toDouble(),
      diskUsage: (json['disk_usage'] as num).toDouble(),
      network: NodeNetworkStats.fromJson(
        json['network'] as Map<String, dynamic>,
      ),
      connections: json['connections'] as int,
      users: json['users'] as int,
      lastUpdated: json['last_updated'] as String,
    );
  }
}

/// Node sync response data
class NodeSyncResponseData {
  final int nodeId;
  final String status;
  final String syncedAt;

  NodeSyncResponseData({
    required this.nodeId,
    required this.status,
    required this.syncedAt,
  });

  factory NodeSyncResponseData.fromJson(Map<String, dynamic> json) {
    return NodeSyncResponseData(
      nodeId: json['node_id'] as int,
      status: json['status'] as String,
      syncedAt: json['synced_at'] as String,
    );
  }
}

/// Node connection test response data
class NodeConnectionTestResponseData {
  final int nodeId;
  final String host;
  final int port;
  final bool reachable;
  final double? responseTime;
  final String? error;
  final String testedAt;

  NodeConnectionTestResponseData({
    required this.nodeId,
    required this.host,
    required this.port,
    required this.reachable,
    this.responseTime,
    this.error,
    required this.testedAt,
  });

  factory NodeConnectionTestResponseData.fromJson(Map<String, dynamic> json) {
    return NodeConnectionTestResponseData(
      nodeId: json['node_id'] as int,
      host: json['host'] as String,
      port: json['port'] as int,
      reachable: json['reachable'] as bool,
      responseTime: (json['response_time'] as num?)?.toDouble(),
      error: json['error'] as String?,
      testedAt: json['tested_at'] as String,
    );
  }
}

/// Node log entry
class NodeLogEntry {
  final String timestamp;
  final String level;
  final String message;

  NodeLogEntry({
    required this.timestamp,
    required this.level,
    required this.message,
  });

  factory NodeLogEntry.fromJson(Map<String, dynamic> json) {
    return NodeLogEntry(
      timestamp: json['timestamp'] as String,
      level: json['level'] as String,
      message: json['message'] as String,
    );
  }
}

/// Node logs response data
class NodeLogsResponseData {
  final int nodeId;
  final String nodeName;
  final List<NodeLogEntry> logs;
  final int total;

  NodeLogsResponseData({
    required this.nodeId,
    required this.nodeName,
    required this.logs,
    required this.total,
  });

  factory NodeLogsResponseData.fromJson(Map<String, dynamic> json) {
    return NodeLogsResponseData(
      nodeId: json['node_id'] as int,
      nodeName: json['node_name'] as String,
      logs: (json['logs'] as List)
          .map((e) => NodeLogEntry.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
    );
  }
}

/// Node bulk action result
class NodeBulkActionResult {
  final int id;
  final bool success;
  final String? error;

  NodeBulkActionResult({
    required this.id,
    required this.success,
    this.error,
  });

  factory NodeBulkActionResult.fromJson(Map<String, dynamic> json) {
    return NodeBulkActionResult(
      id: json['id'] as int,
      success: json['success'] as bool,
      error: json['error'] as String?,
    );
  }
}

/// Node bulk action response
class NodeBulkActionResponse extends ApiSuccessResponse<NodeBulkActionResponseData> {
  NodeBulkActionResponse({required super.data});

  factory NodeBulkActionResponse.fromJson(Map<String, dynamic> json) {
    return NodeBulkActionResponse(
      data: NodeBulkActionResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class NodeBulkActionResponseData {
  final String action;
  final List<NodeBulkActionResult> results;

  NodeBulkActionResponseData({
    required this.action,
    required this.results,
  });

  factory NodeBulkActionResponseData.fromJson(Map<String, dynamic> json) {
    return NodeBulkActionResponseData(
      action: json['action'] as String,
      results: (json['results'] as List)
          .map((e) => NodeBulkActionResult.fromJson(e as Map<String, dynamic>))
          .toList(),
    );
  }
}

/// Response types
class NodeSummaryResponse extends ApiSuccessResponse<NodeSummaryResponseData> {
  NodeSummaryResponse({required super.data});

  factory NodeSummaryResponse.fromJson(Map<String, dynamic> json) {
    return NodeSummaryResponse(
      data: NodeSummaryResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class NodeGetResponse extends ApiSuccessResponse<Node> {
  NodeGetResponse({required super.data});

  factory NodeGetResponse.fromJson(Map<String, dynamic> json) {
    return NodeGetResponse(
      data: Node.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}

class NodeStatsResponse extends ApiSuccessResponse<NodeStatsResponseData> {
  NodeStatsResponse({required super.data});

  factory NodeStatsResponse.fromJson(Map<String, dynamic> json) {
    return NodeStatsResponse(
      data: NodeStatsResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class NodeLogsResponse extends ApiSuccessResponse<NodeLogsResponseData> {
  NodeLogsResponse({required super.data});

  factory NodeLogsResponse.fromJson(Map<String, dynamic> json) {
    return NodeLogsResponse(
      data: NodeLogsResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}