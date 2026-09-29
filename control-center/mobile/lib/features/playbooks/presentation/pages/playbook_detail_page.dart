import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../../core/models/playbook_models.dart';
import '../providers/playbooks_provider.dart';

class PlaybookDetailPage extends ConsumerStatefulWidget {
  final String playbookName;

  const PlaybookDetailPage({super.key, required this.playbookName});

  @override
  ConsumerState<PlaybookDetailPage> createState() => _PlaybookDetailPageState();
}

class _PlaybookDetailPageState extends ConsumerState<PlaybookDetailPage> {
  Playbook? _playbook;
  bool _isLoading = true;
  String? _error;
  final Map<String, TextEditingController> _variableControllers = {};

  @override
  void initState() {
    super.initState();
    _loadPlaybook();
  }

  @override
  void dispose() {
    for (final controller in _variableControllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> _loadPlaybook() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final playbook = await ref.read(playbooksProvider.notifier).getPlaybook(widget.playbookName);

      // Initialize variable controllers
      if (playbook?.variables != null && playbook!.variables!.isNotEmpty) {
        try {
          final varsMap = jsonDecode(playbook.variables!) as Map<String, dynamic>;
          for (final entry in varsMap.entries) {
            final varInfo = entry.value as Map<String, dynamic>;
            final defaultValue = varInfo['default']?.toString() ?? '';
            _variableControllers[entry.key] = TextEditingController(text: defaultValue);
          }
        } catch (_) {
          // Ignore JSON parse errors
        }
      }

      setState(() {
        _playbook = playbook;
        _isLoading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _isLoading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: Text(_playbook?.name ?? 'Loading...'),
        actions: [
          if (_playbook != null)
            IconButton(
              icon: const Icon(Icons.play_arrow),
              tooltip: 'Run Playbook',
              onPressed: () => _showRunDialog(context),
            ),
        ],
      ),
      body: _isLoading
          ? const Center(child: CircularProgressIndicator())
          : _error != null
              ? _buildError()
              : _buildContent(theme),
    );
  }

  Widget _buildError() {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(Icons.error_outline, size: 48, color: Colors.red),
          const SizedBox(height: 16),
          Text('Error: $_error'),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: _loadPlaybook,
            child: const Text('Retry'),
          ),
        ],
      ),
    );
  }

  Widget _buildContent(ThemeData theme) {
    if (_playbook == null) return const SizedBox();

    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header card
          _buildHeaderCard(theme),
          const SizedBox(height: 16),

          // Variables section
          if (_playbook!.variables != null && _playbook!.variables!.isNotEmpty) ...[
            _buildSectionTitle('Variables', theme),
            _buildVariablesCard(theme),
            const SizedBox(height: 16),
          ],

          // Content section
          _buildSectionTitle('YAML Content', theme),
          _buildContentCard(theme),
          const SizedBox(height: 24),

          // Run button
          SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              onPressed: () => _showRunDialog(context),
              icon: const Icon(Icons.play_arrow),
              label: const Text('Run Playbook'),
              style: ElevatedButton.styleFrom(
                padding: const EdgeInsets.symmetric(vertical: 16),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildHeaderCard(ThemeData theme) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                _buildCategoryChip(theme),
                const SizedBox(width: 8),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  decoration: BoxDecoration(
                    color: Colors.grey.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: Text(
                    _playbook!.source ?? 'custom',
                    style: theme.textTheme.bodySmall,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              _playbook!.description ?? 'No description available',
              style: theme.textTheme.bodyMedium,
            ),
            if (_playbook!.author != null) ...[
              const SizedBox(height: 8),
              Text(
                'Author: ${_playbook!.author}',
                style: theme.textTheme.bodySmall?.copyWith(color: Colors.grey),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _buildCategoryChip(ThemeData theme) {
    Color color;
    IconData icon;

    switch (_playbook!.category) {
      case 'security':
        color = Colors.red;
        icon = Icons.security;
        break;
      case 'infrastructure':
        color = Colors.blue;
        icon = Icons.dns;
        break;
      case 'proxy':
        color = Colors.purple;
        icon = Icons.public;
        break;
      case 'maintenance':
        color = Colors.orange;
        icon = Icons.build;
        break;
      case 'ssl':
        color = Colors.green;
        icon = Icons.lock;
        break;
      default:
        color = Colors.grey;
        icon = Icons.code;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 14, color: color),
          const SizedBox(width: 4),
          Text(
            _playbook!.category ?? 'custom',
            style: TextStyle(fontSize: 12, color: color),
          ),
        ],
      ),
    );
  }

  Widget _buildSectionTitle(String title, ThemeData theme) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(
        title,
        style: theme.textTheme.titleMedium?.copyWith(
          fontWeight: FontWeight.bold,
        ),
      ),
    );
  }

  Widget _buildVariablesCard(ThemeData theme) {
    // Parse variables JSON string
    Map<String, dynamic> varsMap = {};
    if (_playbook!.variables != null && _playbook!.variables!.isNotEmpty) {
      try {
        varsMap = jsonDecode(_playbook!.variables!) as Map<String, dynamic>;
      } catch (_) {
        // Ignore JSON parse errors
      }
    }

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: varsMap.entries.map((entry) {
            final varName = entry.key;
            final varInfo = entry.value as Map<String, dynamic>;
            final description = varInfo['description'] as String?;
            final type = varInfo['type'] as String?;

            return Padding(
              padding: const EdgeInsets.only(bottom: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    varName,
                    style: theme.textTheme.bodyMedium?.copyWith(
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                  if (description != null) ...[
                    const SizedBox(height: 4),
                    Text(
                      description,
                      style: theme.textTheme.bodySmall?.copyWith(color: Colors.grey),
                    ),
                  ],
                  const SizedBox(height: 8),
                  TextField(
                    controller: _variableControllers[varName],
                    decoration: InputDecoration(
                      labelText: type ?? 'Value',
                      border: const OutlineInputBorder(),
                      isDense: true,
                    ),
                    keyboardType: type == 'number' ? TextInputType.number : TextInputType.text,
                  ),
                ],
              ),
            );
          }).toList(),
        ),
      ),
    );
  }

  Widget _buildContentCard(ThemeData theme) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(
                  'playbook.yml',
                  style: theme.textTheme.bodySmall?.copyWith(
                    color: Colors.grey,
                    fontFamily: 'monospace',
                  ),
                ),
                IconButton(
                  icon: const Icon(Icons.copy, size: 18),
                  tooltip: 'Copy',
                  onPressed: () {
                    // Copy to clipboard
                  },
                ),
              ],
            ),
            const SizedBox(height: 8),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: theme.brightness == Brightness.dark
                    ? Colors.grey[900]
                    : Colors.grey[100],
                borderRadius: BorderRadius.circular(8),
              ),
              child: Text(
                _playbook!.description ?? 'No description available',
                style: const TextStyle(
                  fontFamily: 'monospace',
                  fontSize: 12,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  void _showRunDialog(BuildContext context) {
    // Get selected nodes for running playbook
    showDialog(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Run Playbook'),
        content: const Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Select target nodes to run this playbook.'),
            SizedBox(height: 16),
            Text('Note: This feature requires node selection UI.'),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () {
              Navigator.pop(context);
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('Playbook execution will be implemented with Tasks feature')),
              );
            },
            child: const Text('Run'),
          ),
        ],
      ),
    );
  }
}