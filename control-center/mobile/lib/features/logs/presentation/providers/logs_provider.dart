import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/providers/api_providers.dart';

/// Log entry model
class LogEntry {
  final String id;
  final String level;
  final String source;
  final String message;
  final DateTime timestamp;
  final Map<String, dynamic>? metadata;

  const LogEntry({
    required this.id,
    required this.level,
    required this.source,
    required this.message,
    required this.timestamp,
    this.metadata,
  });

  factory LogEntry.fromJson(Map<String, dynamic> json) {
    return LogEntry(
      id: json['id']?.toString() ?? '',
      level: json['level'] ?? 'info',
      source: json['source'] ?? 'system',
      message: json['message'] ?? '',
      timestamp: json['timestamp'] != null
          ? DateTime.tryParse(json['timestamp']) ?? DateTime.now()
          : DateTime.now(),
      metadata: json['metadata'] as Map<String, dynamic>?,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'level': level,
      'source': source,
      'message': message,
      'timestamp': timestamp.toIso8601String(),
      'metadata': metadata,
    };
  }
}

/// Logs state
class LogsState {
  final List<LogEntry> logs;
  final bool isLoading;
  final bool isStreaming;
  final String? error;
  final String? levelFilter;
  final String? sourceFilter;
  final String? searchQuery;

  const LogsState({
    this.logs = const [],
    this.isLoading = false,
    this.isStreaming = false,
    this.error,
    this.levelFilter,
    this.sourceFilter,
    this.searchQuery,
  });

  LogsState copyWith({
    List<LogEntry>? logs,
    bool? isLoading,
    bool? isStreaming,
    String? error,
    String? levelFilter,
    String? sourceFilter,
    String? searchQuery,
  }) {
    return LogsState(
      logs: logs ?? this.logs,
      isLoading: isLoading ?? this.isLoading,
      isStreaming: isStreaming ?? this.isStreaming,
      error: error,
      levelFilter: levelFilter ?? this.levelFilter,
      sourceFilter: sourceFilter ?? this.sourceFilter,
      searchQuery: searchQuery ?? this.searchQuery,
    );
  }

  List<LogEntry> get filteredLogs {
    var filtered = logs;

    if (levelFilter != null && levelFilter!.isNotEmpty) {
      filtered = filtered.where((log) => log.level == levelFilter).toList();
    }

    if (sourceFilter != null && sourceFilter!.isNotEmpty) {
      filtered = filtered.where((log) => log.source == sourceFilter).toList();
    }

    if (searchQuery != null && searchQuery!.isNotEmpty) {
      final query = searchQuery!.toLowerCase();
      filtered = filtered.where((log) {
        return log.message.toLowerCase().contains(query) ||
            log.source.toLowerCase().contains(query);
      }).toList();
    }

    return filtered;
  }

  List<LogEntry> get errorLogs => logs.where((log) => log.level == 'error').toList();
  List<LogEntry> get warningLogs => logs.where((log) => log.level == 'warning').toList();
  List<LogEntry> get infoLogs => logs.where((log) => log.level == 'info').toList();

  int get errorCount => errorLogs.length;
  int get warningCount => warningLogs.length;
  int get infoCount => infoLogs.length;

  Set<String> get sources => logs.map((log) => log.source).toSet();
}

/// Provider for LogsState
final logsProvider = NotifierProvider<LogsNotifier, LogsState>(LogsNotifier.new);

/// Logs notifier
class LogsNotifier extends Notifier<LogsState> {
  @override
  LogsState build() => const LogsState();

  /// Load logs from API
  Future<void> loadLogs() async {
    state = state.copyWith(isLoading: true, error: null);

    try {
      // For now, use mock data
      // In production, this would fetch from API
      await Future.delayed(const Duration(milliseconds: 500));

      final mockLogs = _generateMockLogs();
      state = state.copyWith(
        logs: mockLogs,
        isLoading: false,
      );
    } catch (e) {
      state = state.copyWith(
        isLoading: false,
        error: e.toString(),
      );
    }
  }

  /// Add a new log entry
  void addLog(LogEntry log) {
    state = state.copyWith(
      logs: [log, ...state.logs],
    );
  }

  /// Clear all logs
  void clearLogs() {
    state = state.copyWith(logs: []);
  }

  /// Set level filter
  void setLevelFilter(String? level) {
    state = state.copyWith(levelFilter: level);
  }

  /// Set source filter
  void setSourceFilter(String? source) {
    state = state.copyWith(sourceFilter: source);
  }

  /// Set search query
  void setSearchQuery(String? query) {
    state = state.copyWith(searchQuery: query);
  }

  /// Start streaming logs
  void startStreaming() {
    state = state.copyWith(isStreaming: true);
  }

  /// Stop streaming logs
  void stopStreaming() {
    state = state.copyWith(isStreaming: false);
  }

  /// Clear error
  void clearError() {
    state = state.copyWith(error: null);
  }

  List<LogEntry> _generateMockLogs() {
    final now = DateTime.now();
    return [
      LogEntry(
        id: '1',
        level: 'info',
        source: 'system',
        message: 'Application started',
        timestamp: now.subtract(const Duration(minutes: 5)),
      ),
      LogEntry(
        id: '2',
        level: 'info',
        source: 'node.tokyo',
        message: 'Node connected successfully',
        timestamp: now.subtract(const Duration(minutes: 4)),
      ),
      LogEntry(
        id: '3',
        level: 'warning',
        source: 'node.tokyo',
        message: 'High memory usage detected',
        timestamp: now.subtract(const Duration(minutes: 3)),
      ),
      LogEntry(
        id: '4',
        level: 'error',
        source: 'node.sinagpore',
        message: 'Connection timeout',
        timestamp: now.subtract(const Duration(minutes: 2)),
      ),
      LogEntry(
        id: '5',
        level: 'info',
        source: 'system',
        message: 'Health check passed',
        timestamp: now.subtract(const Duration(minutes: 1)),
      ),
    ];
  }
}