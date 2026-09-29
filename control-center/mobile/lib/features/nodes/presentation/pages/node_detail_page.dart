import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';
import '../providers/nodes_provider.dart';

/// Node detail page with stats and management options
class NodeDetailPage extends ConsumerStatefulWidget {
  final String nodeId;

  const NodeDetailPage({super.key, required this.nodeId});

  @override
  ConsumerState<NodeDetailPage> createState() => _NodeDetailPageState();
}

class _NodeDetailPageState extends ConsumerState<NodeDetailPage>
    with SingleTickerProviderStateMixin {
  late TabController _tabController;
  bool _isLoading = false;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 4, vsync: this);
    _loadNodeDetails();
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _loadNodeDetails() async {
    setState(() => _isLoading = true);
    // TODO: Load node details from API
    await Future.delayed(const Duration(seconds: 1));
    setState(() => _isLoading = false);
  }

  @override
  Widget build(BuildContext context) {
    final node = ref.watch(nodeProvider(widget.nodeId));

    if (node == null) {
      return Scaffold(
        appBar: AppBar(title: const Text('Node Not Found')),
        body: const Center(child: Text('The requested node could not be found.')),
      );
    }

    return Scaffold(
      body: Column(
        children: [
          // Header
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(
              color: AppTheme.darkSurface,
              border: Border(bottom: BorderSide(color: AppTheme.darkBorder)),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      width: 60,
                      height: 60,
                      decoration: BoxDecoration(
                        color: AppTheme.primaryColor.withOpacity(0.1),
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: Icon(Icons.dns, color: AppTheme.primaryColor, size: 32),
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Row(
                            children: [
                              Text(
                                node.name,
                                style: const TextStyle(
                                  fontSize: 24,
                                  fontWeight: FontWeight.bold,
                                  color: AppTheme.darkText,
                                ),
                              ),
                              const SizedBox(width: 12),
                              _buildStatusBadge(node.status),
                            ],
                          ),
                          const SizedBox(height: 4),
                          Text(
                            '${node.host}:${node.port}',
                            style: const TextStyle(color: AppTheme.darkTextSecondary),
                          ),
                        ],
                      ),
                    ),
                    _buildQuickActions(node),
                  ],
                ),
                const SizedBox(height: 20),
                // Stats row
                Row(
                  children: [
                    _buildStatCard('Users', '${node.users}', Icons.people, Colors.blue),
                    const SizedBox(width: 12),
                    _buildStatCard('Traffic', _formatBytes(node.traffic), Icons.swap_vert, Colors.green),
                    const SizedBox(width: 12),
                    _buildStatCard('CPU', '${node.cpuUsage?.toStringAsFixed(1) ?? '0'}%', Icons.memory, Colors.orange),
                    const SizedBox(width: 12),
                    _buildStatCard('Memory', '${node.memoryUsage?.toStringAsFixed(1) ?? '0'}%', Icons.storage, Colors.purple),
                  ],
                ),
              ],
            ),
          ),

          // Tabs
          TabBar(
            controller: _tabController,
            labelColor: AppTheme.primaryColor,
            unselectedLabelColor: AppTheme.darkTextSecondary,
            indicatorColor: AppTheme.primaryColor,
            tabs: const [
              Tab(text: 'Overview'),
              Tab(text: 'Config'),
              Tab(text: 'Logs'),
              Tab(text: 'Terminal'),
            ],
          ),

          // Tab content
          Expanded(
            child: TabBarView(
              controller: _tabController,
              children: [
                _buildOverviewTab(node),
                _buildConfigTab(node),
                _buildLogsTab(),
                _buildTerminalTab(),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildStatusBadge(String status) {
    final isOnline = status == 'online';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: isOnline ? Colors.green.withOpacity(0.1) : Colors.red.withOpacity(0.1),
        borderRadius: BorderRadius.circular(20),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 8,
            height: 8,
            decoration: BoxDecoration(
              color: isOnline ? Colors.green : Colors.red,
              shape: BoxShape.circle,
            ),
          ),
          const SizedBox(width: 6),
          Text(
            status.toUpperCase(),
            style: TextStyle(
              color: isOnline ? Colors.green : Colors.red,
              fontSize: 12,
              fontWeight: FontWeight.bold,
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildQuickActions(Node node) {
    final isOnline = node.status == 'online';

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (isOnline) ...[
          IconButton(
            icon: const Icon(Icons.stop, color: Colors.red),
            onPressed: () => _showActionConfirm('Stop', node.id, 'stop'),
            tooltip: 'Stop',
          ),
          IconButton(
            icon: const Icon(Icons.refresh, color: Colors.orange),
            onPressed: () => _showActionConfirm('Restart', node.id, 'restart'),
            tooltip: 'Restart',
          ),
        ] else
          IconButton(
            icon: const Icon(Icons.play_arrow, color: Colors.green),
            onPressed: () => _showActionConfirm('Start', node.id, 'start'),
            tooltip: 'Start',
          ),
        IconButton(
          icon: const Icon(Icons.terminal, color: AppTheme.primaryColor),
          onPressed: () => _tabController.animateTo(3),
          tooltip: 'Terminal',
        ),
        IconButton(
          icon: const Icon(Icons.more_vert),
          onPressed: () => _showMoreOptions(node),
        ),
      ],
    );
  }

  Widget _buildStatCard(String label, String value, IconData icon, Color color) {
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: AppTheme.darkSurface,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: AppTheme.darkBorder),
        ),
        child: Column(
          children: [
            Icon(icon, color: color, size: 20),
            const SizedBox(height: 8),
            Text(
              value,
              style: TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.bold,
                color: color,
              ),
            ),
            Text(
              label,
              style: const TextStyle(fontSize: 12, color: AppTheme.darkTextSecondary),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildOverviewTab(Node node) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Info cards
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Information',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
                ),
                const SizedBox(height: 16),
                _buildInfoRow('Type', node.type.toUpperCase()),
                _buildInfoRow('Version', node.version ?? 'Unknown'),
                _buildInfoRow('Last Seen', node.lastSeen?.toString() ?? 'Never'),
              ],
            ),
          ),
        ),

        const SizedBox(height: 16),

        // Quick stats
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Performance',
                  style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
                ),
                const SizedBox(height: 16),
                _buildProgressBar('CPU Usage', node.cpuUsage ?? 0, Colors.orange),
                const SizedBox(height: 12),
                _buildProgressBar('Memory Usage', node.memoryUsage ?? 0, Colors.purple),
                const SizedBox(height: 12),
                _buildProgressBar('Disk Usage', 45.0, Colors.blue), // Placeholder
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildConfigTab(Node node) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Text(
                      'Configuration',
                      style: TextStyle(fontSize: 16, fontWeight: FontWeight.bold),
                    ),
                    const Spacer(),
                    TextButton.icon(
                      icon: const Icon(Icons.edit, size: 18),
                      label: const Text('Edit'),
                      onPressed: () {},
                    ),
                  ],
                ),
                const SizedBox(height: 16),
                const SelectableText(
                  '''
{
  "server": {
    "host": "0.0.0.0",
    "port": 443
  },
  "log": {
    "level": "info"
  }
}
                  ''',
                  style: TextStyle(fontFamily: 'monospace', fontSize: 12),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildLogsTab() {
    return Column(
      children: [
        Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: AppTheme.darkSurface,
            border: Border(bottom: BorderSide(color: AppTheme.darkBorder)),
          ),
          child: Row(
            children: [
              const Expanded(
                child: TextField(
                  decoration: InputDecoration(
                    hintText: 'Filter logs...',
                    prefixIcon: Icon(Icons.search),
                    border: OutlineInputBorder(),
                    contentPadding: EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                  ),
                ),
              ),
              const SizedBox(width: 8),
              IconButton(icon: const Icon(Icons.refresh), onPressed: () {}),
              IconButton(icon: const Icon(Icons.download), onPressed: () {}),
            ],
          ),
        ),
        Expanded(
          child: Container(
            color: Colors.black,
            padding: const EdgeInsets.all(8),
            child: ListView(
              children: const [
                Text(
                  '[2024-03-18 10:30:15] INFO: Server started on port 443',
                  style: TextStyle(fontFamily: 'monospace', fontSize: 12, color: Colors.green),
                ),
                Text(
                  '[2024-03-18 10:30:16] INFO: Connection established from 192.168.1.100',
                  style: TextStyle(fontFamily: 'monospace', fontSize: 12, color: Colors.white),
                ),
                Text(
                  '[2024-03-18 10:30:20] WARN: High memory usage detected',
                  style: TextStyle(fontFamily: 'monospace', fontSize: 12, color: Colors.orange),
                ),
                Text(
                  '[2024-03-18 10:30:25] ERROR: Connection timeout',
                  style: TextStyle(fontFamily: 'monospace', fontSize: 12, color: Colors.red),
                ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildTerminalTab() {
    return Column(
      children: [
        Expanded(
          child: Container(
            color: Colors.black,
            padding: const EdgeInsets.all(8),
            child: const Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Welcome to AnixOps Terminal',
                  style: TextStyle(fontFamily: 'monospace', color: Colors.green),
                ),
                SizedBox(height: 8),
                Text(
                  '\$ _',
                  style: TextStyle(fontFamily: 'monospace', color: Colors.white),
                ),
              ],
            ),
          ),
        ),
        Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(
            color: AppTheme.darkSurface,
            border: Border(top: BorderSide(color: AppTheme.darkBorder)),
          ),
          child: Row(
            children: [
              const Text('\$', style: TextStyle(fontFamily: 'monospace')),
              const SizedBox(width: 8),
              const Expanded(
                child: TextField(
                  decoration: InputDecoration(
                    hintText: 'Enter command...',
                    border: InputBorder.none,
                  ),
                  style: TextStyle(fontFamily: 'monospace'),
                ),
              ),
              IconButton(icon: const Icon(Icons.send), onPressed: () {}),
            ],
          ),
        ),
      ],
    );
  }

  Widget _buildInfoRow(String label, String value) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 8),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(color: AppTheme.darkTextSecondary)),
          Text(value, style: const TextStyle(color: AppTheme.darkText)),
        ],
      ),
    );
  }

  Widget _buildProgressBar(String label, double value, Color color) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(label),
            Text('${value.toStringAsFixed(1)}%'),
          ],
        ),
        const SizedBox(height: 8),
        LinearProgressIndicator(
          value: value / 100,
          backgroundColor: AppTheme.darkBorder,
          valueColor: AlwaysStoppedAnimation(color),
        ),
      ],
    );
  }

  String _formatBytes(int bytes) {
    if (bytes == 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    final i = (bytes.bitLength - 1) ~/ 10;
    return '${(bytes / (1 << (i * 10))).toStringAsFixed(1)} ${sizes[i]}';
  }

  void _showActionConfirm(String action, String nodeId, String actionType) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: Text('$action Node'),
        content: Text('Are you sure you want to $action this node?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () async {
              Navigator.pop(context);
              try {
                switch (actionType) {
                  case 'start':
                    await ref.read(nodesProvider.notifier).startNode(nodeId);
                    break;
                  case 'stop':
                    await ref.read(nodesProvider.notifier).stopNode(nodeId);
                    break;
                  case 'restart':
                    await ref.read(nodesProvider.notifier).restartNode(nodeId);
                    break;
                }
              } catch (e) {
                if (mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(content: Text('Failed to $action: $e')),
                  );
                }
              }
            },
            style: ElevatedButton.styleFrom(
              backgroundColor: actionType == 'stop' ? Colors.red : AppTheme.primaryColor,
            ),
            child: Text(action),
          ),
        ],
      ),
    );
  }

  void _showMoreOptions(Node node) {
    showModalBottomSheet(
      context: context,
      builder: (context) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.sync),
              title: const Text('Sync Configuration'),
              onTap: () {
                Navigator.pop(context);
                ScaffoldMessenger.of(context).showSnackBar(
                  const SnackBar(content: Text('Syncing configuration...')),
                );
              },
            ),
            ListTile(
              leading: const Icon(Icons.edit),
              title: const Text('Edit Node'),
              onTap: () {
                Navigator.pop(context);
              },
            ),
            ListTile(
              leading: const Icon(Icons.folder_outlined),
              title: const Text('Change Group'),
              onTap: () {
                Navigator.pop(context);
              },
            ),
            ListTile(
              leading: const Icon(Icons.delete, color: Colors.red),
              title: const Text('Delete Node', style: TextStyle(color: Colors.red)),
              onTap: () {
                Navigator.pop(context);
                _showDeleteConfirm(node);
              },
            ),
          ],
        ),
      ),
    );
  }

  void _showDeleteConfirm(Node node) {
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete Node'),
        content: Text('Are you sure you want to delete "${node.name}"? This action cannot be undone.'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () async {
              Navigator.pop(context);
              try {
                await ref.read(nodesProvider.notifier).deleteNode(node.id);
                if (mounted) {
                  Navigator.pop(context);
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('Node deleted successfully')),
                  );
                }
              } catch (e) {
                if (mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(content: Text('Failed to delete: $e')),
                  );
                }
              }
            },
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Delete'),
          ),
        ],
      ),
    );
  }
}