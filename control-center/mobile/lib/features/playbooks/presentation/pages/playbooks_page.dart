import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../../../../core/models/playbook_models.dart';
import '../providers/playbooks_provider.dart';
import 'playbook_detail_page.dart';

class PlaybooksPage extends ConsumerWidget {
  const PlaybooksPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(playbooksProvider);
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Playbooks'),
        actions: [
          IconButton(
            icon: const Icon(Icons.add),
            tooltip: 'Upload Playbook',
            onPressed: () => _showUploadDialog(context, ref),
          ),
        ],
      ),
      body: state.isLoading
          ? const Center(child: CircularProgressIndicator())
          : state.error != null
              ? _buildError(context, state.error!, ref)
              : _buildContent(context, ref, state, theme),
    );
  }

  Widget _buildError(BuildContext context, String error, WidgetRef ref) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          const Icon(Icons.error_outline, size: 48, color: Colors.red),
          const SizedBox(height: 16),
          Text('Error: $error'),
          const SizedBox(height: 16),
          ElevatedButton(
            onPressed: () => ref.read(playbooksProvider.notifier).loadAll(),
            child: const Text('Retry'),
          ),
        ],
      ),
    );
  }

  Widget _buildContent(
    BuildContext context,
    WidgetRef ref,
    PlaybooksState state,
    ThemeData theme,
  ) {
    return Column(
      children: [
        // Category filter chips
        _buildCategoryFilter(context, ref, state, theme),

        // Playbooks list
        Expanded(
          child: state.filteredPlaybooks.isEmpty
              ? _buildEmptyState(context)
              : _buildPlaybooksList(context, ref, state, theme),
        ),
      ],
    );
  }

  Widget _buildCategoryFilter(
    BuildContext context,
    WidgetRef ref,
    PlaybooksState state,
    ThemeData theme,
  ) {
    return Container(
      height: 50,
      padding: const EdgeInsets.symmetric(horizontal: 8),
      child: ListView(
        scrollDirection: Axis.horizontal,
        children: [
          FilterChip(
            label: const Text('All'),
            selected: state.selectedCategory == null || state.selectedCategory == 'all',
            onSelected: (_) => ref.read(playbooksProvider.notifier).setCategory(null),
          ),
          const SizedBox(width: 8),
          ...state.categories.map((cat) => Padding(
            padding: const EdgeInsets.only(right: 8),
            child: FilterChip(
              label: Text(cat.name),
              selected: state.selectedCategory == cat.id,
              onSelected: (_) => ref.read(playbooksProvider.notifier).setCategory(cat.id),
            ),
          )),
        ],
      ),
    );
  }

  Widget _buildEmptyState(BuildContext context) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(Icons.play_circle_outline, size: 64, color: Colors.grey[600]),
          const SizedBox(height: 16),
          Text(
            'No playbooks yet',
            style: Theme.of(context).textTheme.titleLarge,
          ),
          const SizedBox(height: 8),
          const Text('Tap + to upload your first playbook'),
        ],
      ),
    );
  }

  Widget _buildPlaybooksList(
    BuildContext context,
    WidgetRef ref,
    PlaybooksState state,
    ThemeData theme,
  ) {
    return ListView.builder(
      padding: const EdgeInsets.all(16),
      itemCount: state.filteredPlaybooks.length,
      itemBuilder: (context, index) {
        final playbook = state.filteredPlaybooks[index];
        return _PlaybookCard(
          playbook: playbook,
          onTap: () => _openPlaybookDetail(context, playbook),
          onDelete: playbook.source == 'custom'
              ? () => _deletePlaybook(context, ref, playbook)
              : null,
        );
      },
    );
  }

  void _openPlaybookDetail(BuildContext context, Playbook playbook) {
    Navigator.push(
      context,
      MaterialPageRoute(
        builder: (context) => PlaybookDetailPage(playbookName: playbook.name),
      ),
    );
  }

  Future<void> _deletePlaybook(BuildContext context, WidgetRef ref, Playbook playbook) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete Playbook'),
        content: Text('Are you sure you want to delete "${playbook.name}"?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('Cancel'),
          ),
          ElevatedButton(
            onPressed: () => Navigator.pop(context, true),
            style: ElevatedButton.styleFrom(backgroundColor: Colors.red),
            child: const Text('Delete'),
          ),
        ],
      ),
    );

    if (confirmed == true && context.mounted) {
      final success = await ref.read(playbooksProvider.notifier).deletePlaybook(playbook.name);
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(success ? 'Playbook deleted' : 'Delete failed'),
          ),
        );
      }
    }
  }

  void _showUploadDialog(BuildContext context, WidgetRef ref) {
    final nameController = TextEditingController();
    final contentController = TextEditingController();
    final descriptionController = TextEditingController();
    String? selectedCategory = 'custom';

    showDialog(
      context: context,
      builder: (context) => StatefulBuilder(
        builder: (context, setState) => AlertDialog(
          title: const Text('Upload Playbook'),
          content: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                TextField(
                  controller: nameController,
                  decoration: const InputDecoration(
                    labelText: 'Name',
                    hintText: 'my-playbook',
                  ),
                ),
                const SizedBox(height: 16),
                TextField(
                  controller: descriptionController,
                  decoration: const InputDecoration(
                    labelText: 'Description (optional)',
                  ),
                ),
                const SizedBox(height: 16),
                DropdownButtonFormField<String>(
                  value: selectedCategory,
                  decoration: const InputDecoration(labelText: 'Category'),
                  items: const [
                    DropdownMenuItem(value: 'custom', child: Text('Custom')),
                    DropdownMenuItem(value: 'security', child: Text('Security')),
                    DropdownMenuItem(value: 'infrastructure', child: Text('Infrastructure')),
                    DropdownMenuItem(value: 'maintenance', child: Text('Maintenance')),
                  ],
                  onChanged: (value) => setState(() => selectedCategory = value),
                ),
                const SizedBox(height: 16),
                TextField(
                  controller: contentController,
                  decoration: const InputDecoration(
                    labelText: 'YAML Content',
                    alignLabelWithHint: true,
                  ),
                  maxLines: 10,
                ),
              ],
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(context),
              child: const Text('Cancel'),
            ),
            ElevatedButton(
              onPressed: () async {
                if (nameController.text.isEmpty || contentController.text.isEmpty) {
                  return;
                }

                Navigator.pop(context);
                final success = await ref.read(playbooksProvider.notifier).uploadPlaybook(
                      name: nameController.text,
                      storageKey: 'uploads/${nameController.text}.yml',
                      description: descriptionController.text,
                      category: selectedCategory,
                    );

                if (context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    SnackBar(
                      content: Text(success ? 'Playbook uploaded' : 'Upload failed'),
                    ),
                  );
                }
              },
              child: const Text('Upload'),
            ),
          ],
        ),
      ),
    );
  }
}

class _PlaybookCard extends StatelessWidget {
  final Playbook playbook;
  final VoidCallback onTap;
  final VoidCallback? onDelete;

  const _PlaybookCard({
    required this.playbook,
    required this.onTap,
    this.onDelete,
  });

  IconData _getCategoryIcon() {
    switch (playbook.category) {
      case 'security':
        return Icons.security;
      case 'infrastructure':
        return Icons.dns;
      case 'proxy':
        return Icons.public;
      case 'maintenance':
        return Icons.build;
      case 'ssl':
        return Icons.lock;
      default:
        return Icons.code;
    }
  }

  Color _getCategoryColor() {
    switch (playbook.category) {
      case 'security':
        return Colors.red;
      case 'infrastructure':
        return Colors.blue;
      case 'proxy':
        return Colors.purple;
      case 'maintenance':
        return Colors.orange;
      case 'ssl':
        return Colors.green;
      default:
        return Colors.grey;
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              Container(
                width: 48,
                height: 48,
                decoration: BoxDecoration(
                  color: _getCategoryColor().withValues(alpha: 0.1),
                  borderRadius: BorderRadius.circular(12),
                ),
                child: Icon(
                  _getCategoryIcon(),
                  color: _getCategoryColor(),
                ),
              ),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            playbook.name,
                            style: theme.textTheme.titleMedium?.copyWith(
                              fontWeight: FontWeight.bold,
                            ),
                          ),
                        ),
                        if (playbook.source == 'built-in')
                          Container(
                            padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                            decoration: BoxDecoration(
                              color: theme.colorScheme.primary.withValues(alpha: 0.1),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: Text(
                              'Built-in',
                              style: TextStyle(
                                fontSize: 10,
                                color: theme.colorScheme.primary,
                              ),
                            ),
                          ),
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      playbook.description ?? 'No description',
                      style: theme.textTheme.bodySmall,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ),
              ),
              if (onDelete != null)
                IconButton(
                  icon: const Icon(Icons.delete_outline),
                  onPressed: onDelete,
                ),
            ],
          ),
        ),
      ),
    );
  }
}