import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/logs_provider.dart';

class LogsPage extends ConsumerStatefulWidget {
  const LogsPage({super.key});

  @override
  ConsumerState<LogsPage> createState() => _LogsPageState();
}

class _LogsPageState extends ConsumerState<LogsPage> {
  final _searchController = TextEditingController();
  String? _selectedLevel;
  String? _selectedSource;

  @override
  void initState() {
    super.initState();
    Future.microtask(() => ref.read(logsProvider.notifier).loadLogs());
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final logsState = ref.watch(logsProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Logs'),
        actions: [
          IconButton(
            icon: Icon(
              logsState.isStreaming ? Icons.pause : Icons.play_arrow,
            ),
            onPressed: () {
              if (logsState.isStreaming) {
                ref.read(logsProvider.notifier).stopStreaming();
              } else {
                ref.read(logsProvider.notifier).startStreaming();
              }
            },
            tooltip: logsState.isStreaming ? 'Stop Streaming' : 'Start Streaming',
          ),
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.read(logsProvider.notifier).loadLogs(),
          ),
          IconButton(
            icon: const Icon(Icons.delete_outline),
            onPressed: () => _showClearLogsDialog(context),
          ),
        ],
      ),
      body: Column(
        children: [
          // Stats Bar
          _buildStatsBar(logsState),

          // Filters
          _buildFilters(logsState),

          // Logs List
          Expanded(
            child: logsState.isLoading
                ? const Center(child: CircularProgressIndicator())
                : logsState.error != null
                    ? _buildErrorState(logsState.error!)
                    : logsState.filteredLogs.isEmpty
                        ? const Center(child: Text('No logs found'))
                        : _buildLogsList(logsState.filteredLogs),
          ),
        ],
      ),
    );
  }

  Widget _buildStatsBar(LogsState state) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.surfaceContainerHighest,
        border: Border(
          bottom: BorderSide(
            color: Theme.of(context).dividerColor,
          ),
        ),
      ),
      child: Row(
        children: [
          _buildStatChip('Total', state.logs.length, Colors.blue),
          const SizedBox(width: 12),
          _buildStatChip('Errors', state.errorCount, Colors.red),
          const SizedBox(width: 12),
          _buildStatChip('Warnings', state.warningCount, Colors.orange),
          const SizedBox(width: 12),
          _buildStatChip('Info', state.infoCount, Colors.green),
          if (state.isStreaming) ...[
            const Spacer(),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                color: Colors.green.withAlpha(30),
                borderRadius: BorderRadius.circular(12),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Container(
                    width: 8,
                    height: 8,
                    decoration: const BoxDecoration(
                      color: Colors.green,
                      shape: BoxShape.circle,
                    ),
                  ),
                  const SizedBox(width: 6),
                  const Text(
                    'Streaming',
                    style: TextStyle(
                      color: Colors.green,
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                ],
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildStatChip(String label, int count, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
      decoration: BoxDecoration(
        color: color.withAlpha(30),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        '$label: $count',
        style: TextStyle(
          color: color,
          fontSize: 12,
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }

  Widget _buildFilters(LogsState state) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        children: [
          // Search Bar
          TextField(
            controller: _searchController,
            decoration: InputDecoration(
              hintText: 'Search logs...',
              prefixIcon: const Icon(Icons.search),
              suffixIcon: _searchController.text.isNotEmpty
                  ? IconButton(
                      icon: const Icon(Icons.clear),
                      onPressed: () {
                        _searchController.clear();
                        ref.read(logsProvider.notifier).setSearchQuery(null);
                        setState(() {});
                      },
                    )
                  : null,
              border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(12),
              ),
              contentPadding: const EdgeInsets.symmetric(horizontal: 16),
            ),
            onChanged: (value) {
              ref.read(logsProvider.notifier).setSearchQuery(value.isEmpty ? null : value);
              setState(() {});
            },
          ),
          const SizedBox(height: 12),
          // Filter Chips
          Row(
            children: [
              // Level Filter
              Expanded(
                child: DropdownButtonFormField<String>(
                  value: _selectedLevel,
                  decoration: InputDecoration(
                    labelText: 'Level',
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                    contentPadding: const EdgeInsets.symmetric(horizontal: 12),
                  ),
                  items: [
                    const DropdownMenuItem(value: null, child: Text('All Levels')),
                    ...['error', 'warning', 'info', 'debug'].map(
                      (level) => DropdownMenuItem(value: level, child: Text(level.toUpperCase())),
                    ),
                  ],
                  onChanged: (value) {
                    setState(() => _selectedLevel = value);
                    ref.read(logsProvider.notifier).setLevelFilter(value);
                  },
                ),
              ),
              const SizedBox(width: 12),
              // Source Filter
              Expanded(
                child: DropdownButtonFormField<String>(
                  value: _selectedSource,
                  decoration: InputDecoration(
                    labelText: 'Source',
                    border: OutlineInputBorder(
                      borderRadius: BorderRadius.circular(12),
                    ),
                    contentPadding: const EdgeInsets.symmetric(horizontal: 12),
                  ),
                  items: [
                    const DropdownMenuItem(value: null, child: Text('All Sources')),
                    ...state.sources.map(
                      (source) => DropdownMenuItem(value: source, child: Text(source)),
                    ),
                  ],
                  onChanged: (value) {
                    setState(() => _selectedSource = value);
                    ref.read(logsProvider.notifier).setSourceFilter(value);
                  },
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildErrorState(String error) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(Icons.error_outline, size: 48, color: Colors.red),
          const SizedBox(height: 16),
          Text('Error: $error'),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: () => ref.read(logsProvider.notifier).loadLogs(),
            child: const Text('Retry'),
          ),
        ],
      ),
    );
  }

  Widget _buildLogsList(List<LogEntry> logs) {
    return ListView.builder(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      itemCount: logs.length,
      itemBuilder: (context, index) {
        final log = logs[index];
        return _LogCard(
          log: log,
          onTap: () => _showLogDetailDialog(context, log),
        );
      },
    );
  }

  void _showClearLogsDialog(BuildContext context) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Clear Logs'),
        content: const Text('Are you sure you want to clear all logs?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () {
              ref.read(logsProvider.notifier).clearLogs();
              Navigator.pop(context);
            },
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: const Text('Clear'),
          ),
        ],
      ),
    );
  }

  void _showLogDetailDialog(BuildContext context, LogEntry log) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: Row(
          children: [
            _getLevelIcon(log.level),
            const SizedBox(width: 8),
            Text(log.source),
          ],
        ),
        content: SizedBox(
          width: 400,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _DetailRow(label: 'Level', value: log.level.toUpperCase()),
              _DetailRow(label: 'Time', value: _formatDateTime(log.timestamp)),
              _DetailRow(label: 'Message', value: log.message),
              if (log.metadata != null) ...[
                const SizedBox(height: 12),
                const Text(
                  'Metadata:',
                  style: TextStyle(fontWeight: FontWeight.bold),
                ),
                const SizedBox(height: 4),
                Container(
                  padding: const EdgeInsets.all(8),
                  decoration: BoxDecoration(
                    color: Theme.of(context).colorScheme.surfaceContainerHighest,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Text(
                    log.metadata.toString(),
                    style: const TextStyle(fontFamily: 'monospace', fontSize: 12),
                  ),
                ),
              ],
            ],
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  Icon _getLevelIcon(String level) {
    switch (level) {
      case 'error':
        return const Icon(Icons.error, color: Colors.red);
      case 'warning':
        return const Icon(Icons.warning, color: Colors.orange);
      case 'debug':
        return const Icon(Icons.bug_report, color: Colors.grey);
      default:
        return const Icon(Icons.info, color: Colors.blue);
    }
  }

  String _formatDateTime(DateTime dt) {
    return '${dt.year}-${dt.month.toString().padLeft(2, '0')}-${dt.day.toString().padLeft(2, '0')} '
        '${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}:${dt.second.toString().padLeft(2, '0')}';
  }
}

class _LogCard extends StatelessWidget {
  final LogEntry log;
  final VoidCallback onTap;

  const _LogCard({
    required this.log,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.symmetric(vertical: 4),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _getLevelIcon(log.level),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                          decoration: BoxDecoration(
                            color: _getLevelColor(log.level).withAlpha(30),
                            borderRadius: BorderRadius.circular(4),
                          ),
                          child: Text(
                            log.level.toUpperCase(),
                            style: TextStyle(
                              fontSize: 10,
                              fontWeight: FontWeight.bold,
                              color: _getLevelColor(log.level),
                            ),
                          ),
                        ),
                        const SizedBox(width: 8),
                        Text(
                          log.source,
                          style: const TextStyle(
                            fontWeight: FontWeight.w500,
                            fontSize: 12,
                          ),
                        ),
                        const Spacer(),
                        Text(
                          _formatTime(log.timestamp),
                          style: TextStyle(
                            fontSize: 11,
                            color: Colors.grey[600],
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 6),
                    Text(
                      log.message,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontSize: 13),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Icon _getLevelIcon(String level) {
    switch (level) {
      case 'error':
        return const Icon(Icons.error, color: Colors.red, size: 20);
      case 'warning':
        return const Icon(Icons.warning, color: Colors.orange, size: 20);
      case 'debug':
        return const Icon(Icons.bug_report, color: Colors.grey, size: 20);
      default:
        return const Icon(Icons.info, color: Colors.blue, size: 20);
    }
  }

  Color _getLevelColor(String level) {
    switch (level) {
      case 'error':
        return Colors.red;
      case 'warning':
        return Colors.orange;
      case 'debug':
        return Colors.grey;
      default:
        return Colors.blue;
    }
  }

  String _formatTime(DateTime dt) {
    final now = DateTime.now();
    final diff = now.difference(dt);

    if (diff.inMinutes < 1) {
      return 'Just now';
    } else if (diff.inMinutes < 60) {
      return '${diff.inMinutes}m ago';
    } else if (diff.inHours < 24) {
      return '${diff.inHours}h ago';
    } else {
      return '${dt.month}/${dt.day} ${dt.hour.toString().padLeft(2, '0')}:${dt.minute.toString().padLeft(2, '0')}';
    }
  }
}

class _DetailRow extends StatelessWidget {
  final String label;
  final String value;

  const _DetailRow({
    required this.label,
    required this.value,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          SizedBox(
            width: 70,
            child: Text(
              '$label:',
              style: const TextStyle(fontWeight: FontWeight.w500),
            ),
          ),
          Expanded(
            child: Text(value),
          ),
        ],
      ),
    );
  }
}