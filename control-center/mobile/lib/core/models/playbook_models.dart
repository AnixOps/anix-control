// Playbook models matching the backend contract
// See: control-center/workers/src/types.ts

import 'package:anixops_mobile/core/models/api_response.dart';

/// Playbook entity
class Playbook {
  final int id;
  final String name;
  final String storageKey;
  final String? description;
  final String? category;
  final String? source;
  final String? githubRepo;
  final String? githubPath;
  final String? version;
  final String? variables;
  final String? author;
  final String? tags;
  final String createdAt;
  final String updatedAt;

  Playbook({
    required this.id,
    required this.name,
    required this.storageKey,
    this.description,
    this.category,
    this.source,
    this.githubRepo,
    this.githubPath,
    this.version,
    this.variables,
    this.author,
    this.tags,
    required this.createdAt,
    required this.updatedAt,
  });

  factory Playbook.fromJson(Map<String, dynamic> json) {
    return Playbook(
      id: json['id'] as int,
      name: json['name'] as String,
      storageKey: json['storage_key'] as String,
      description: json['description'] as String?,
      category: json['category'] as String?,
      source: json['source'] as String?,
      githubRepo: json['github_repo'] as String?,
      githubPath: json['github_path'] as String?,
      version: json['version'] as String?,
      variables: json['variables'] as String?,
      author: json['author'] as String?,
      tags: json['tags'] as String?,
      createdAt: json['created_at'] as String,
      updatedAt: json['updated_at'] as String,
    );
  }
}

/// Playbook list response data
class PlaybookListResponseData {
  final List<Playbook> items;
  final int total;
  final int page;
  final int perPage;
  final int totalPages;

  PlaybookListResponseData({
    required this.items,
    required this.total,
    required this.page,
    required this.perPage,
    required this.totalPages,
  });

  factory PlaybookListResponseData.fromJson(Map<String, dynamic> json) {
    return PlaybookListResponseData(
      items: (json['items'] as List)
          .map((e) => Playbook.fromJson(e as Map<String, dynamic>))
          .toList(),
      total: json['total'] as int,
      page: json['page'] as int,
      perPage: json['per_page'] as int,
      totalPages: json['total_pages'] as int,
    );
  }
}

/// Playbook run request
class PlaybookRunRequest {
  final int playbookId;
  final List<int>? targetNodeIds;
  final Map<String, dynamic>? variables;

  PlaybookRunRequest({
    required this.playbookId,
    this.targetNodeIds,
    this.variables,
  });

  Map<String, dynamic> toJson() => {
        'playbook_id': playbookId,
        if (targetNodeIds != null) 'target_node_ids': targetNodeIds,
        if (variables != null) 'variables': variables,
      };
}

/// Response types
class PlaybookListResponse extends ApiSuccessResponse<PlaybookListResponseData> {
  PlaybookListResponse({required super.data});

  factory PlaybookListResponse.fromJson(Map<String, dynamic> json) {
    return PlaybookListResponse(
      data: PlaybookListResponseData.fromJson(
        json['data'] as Map<String, dynamic>,
      ),
    );
  }
}

class PlaybookDetailResponse extends ApiSuccessResponse<Playbook> {
  PlaybookDetailResponse({required super.data});

  factory PlaybookDetailResponse.fromJson(Map<String, dynamic> json) {
    return PlaybookDetailResponse(
      data: Playbook.fromJson(json['data'] as Map<String, dynamic>),
    );
  }
}