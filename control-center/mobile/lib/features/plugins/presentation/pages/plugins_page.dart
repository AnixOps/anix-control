import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/core/theme/app_theme.dart';

import '../providers/plugins_provider.dart';

// Plugins Page
class PluginsPage extends ConsumerStatefulWidget {
  const PluginsPage({super.key});

  @override
  ConsumerState<PluginsPage> createState() => _PluginsPageState();
}

class _PluginsPageState extends ConsumerState<PluginsPage> {
  @override
  void initState() {
    super.initState();
    Future.microtask(() => ref.read(pluginsProvider.notifier).fetchPlugins());
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(pluginsProvider);
    final theme = Theme.of(context);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Plugins'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.read(pluginsProvider.notifier).fetchPlugins(),
          ),
        ],
      ),
      body: Column(
        children: [
          // Stats Row
          Padding(
            padding: const EdgeInsets.all(16),
            child: Row(
              children: [
                _buildStatCard('Total', state.plugins.length.toString(), Colors.blue, theme),
                const SizedBox(width: 8),
                _buildStatCard('Running', state.runningCount.toString(), Colors.green, theme),
                const SizedBox(width: 8),
                _buildStatCard('Enabled', state.enabledCount.toString(), Colors.purple, theme),
              ],
            ),
          ),
          // Search Bar
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: TextField(
              onChanged: (value) => ref.read(pluginsProvider.notifier).setSearch(value),
              decoration: InputDecoration(
                hintText: 'Search plugins...',
                prefixIcon: const Icon(Icons.search),
                filled: true,
                fillColor: theme.colorScheme.surface,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(12),
                  borderSide: BorderSide.none,
                ),
                contentPadding: EdgeInsets.zero,
              ),
            ),
          ),
          const SizedBox(height: 8),
          // Plugins List
          Expanded(
            child: state.loading
                ? const Center(child: CircularProgressIndicator())
                : state.filteredPlugins.isEmpty
                    ? Center(
                        child: Column(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            Icon(
                              Icons.extension_outlined,
                              size: 64,
                              color: theme.colorScheme.onSurfaceVariant,
                            ),
                            const SizedBox(height: 16),
                            Text(
                              'No plugins found',
                              style: theme.textTheme.bodyLarge?.copyWith(
                                color: theme.colorScheme.onSurfaceVariant,
                              ),
                            ),
                          ],
                        ),
                      )
                    : RefreshIndicator(
                        onRefresh: () => ref.read(pluginsProvider.notifier).fetchPlugins(),
                        child: ListView.builder(
                          padding: const EdgeInsets.all(16),
                          itemCount: state.filteredPlugins.length,
                          itemBuilder: (context, index) {
                            final plugin = state.filteredPlugins[index];
                            return _PluginCard(
                              plugin: plugin,
                              isExecuting: state.executing,
                              onStart: () => ref.read(pluginsProvider.notifier).startPlugin(plugin.name),
                              onStop: () => ref.read(pluginsProvider.notifier).stopPlugin(plugin.name),
                              onRestart: () => ref.read(pluginsProvider.notifier).restartPlugin(plugin.name),
                              onEnable: () => ref.read(pluginsProvider.notifier).enablePlugin(plugin.name),
                              onDisable: () => ref.read(pluginsProvider.notifier).disablePlugin(plugin.name),
                            );
                          },
                        ),
                      ),
          ),
        ],
      ),
    );
  }

  Widget _buildStatCard(String label, String value, Color color, ThemeData theme) {
    return Expanded(
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: color.withOpacity(0.1),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: color.withOpacity(0.3)),
        ),
        child: Column(
          children: [
            Text(
              value,
              style: theme.textTheme.headlineSmall?.copyWith(
                fontWeight: FontWeight.bold,
                color: color,
              ),
            ),
            Text(
              label,
              style: theme.textTheme.bodySmall?.copyWith(
                color: theme.colorScheme.onSurfaceVariant,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PluginCard extends StatelessWidget {
  final Plugin plugin;
  final bool isExecuting;
  final VoidCallback? onStart;
  final VoidCallback? onStop;
  final VoidCallback? onRestart;
  final VoidCallback? onEnable;
  final VoidCallback? onDisable;

  const _PluginCard({
    required this.plugin,
    this.isExecuting = false,
    this.onStart,
    this.onStop,
    this.onRestart,
    this.onEnable,
    this.onDisable,
  });

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Container(
                  width: 48,
                  height: 48,
                  decoration: BoxDecoration(
                    color: plugin.enabled
                        ? theme.colorScheme.primary.withOpacity(0.1)
                        : theme.colorScheme.surfaceVariant,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: Icon(
                    Icons.extension,
                    color: plugin.enabled
                        ? theme.colorScheme.primary
                        : theme.colorScheme.onSurfaceVariant,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Row(
                        children: [
                          Expanded(
                            child: Text(
                              plugin.displayName,
                              style: theme.textTheme.titleMedium?.copyWith(
                                fontWeight: FontWeight.bold,
                              ),
                            ),
                          ),
                          Text(
                            'v${plugin.version}',
                            style: theme.textTheme.bodySmall?.copyWith(
                              color: theme.colorScheme.onSurfaceVariant,
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 4),
                      Row(
                        children: [
                          Container(
                            width: 8,
                            height: 8,
                            decoration: BoxDecoration(
                              color: plugin.isRunning ? Colors.green : (plugin.isError ? Colors.red : Colors.orange),
                              shape: BoxShape.circle,
                            ),
                          ),
                          const SizedBox(width: 6),
                          Text(
                            plugin.status,
                            style: theme.textTheme.bodySmall?.copyWith(
                              color: plugin.isRunning ? Colors.green : (plugin.isError ? Colors.red : Colors.orange),
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
                Switch(
                  value: plugin.enabled,
                  onChanged: isExecuting ? null : (_) {
                    if (plugin.enabled) {
                      onDisable?.call();
                    } else {
                      onEnable?.call();
                    }
                  },
                ),
              ],
            ),
            if (plugin.description != null) ...[
              const SizedBox(height: 12),
              Text(
                plugin.description!,
                style: theme.textTheme.bodyMedium?.copyWith(
                  color: theme.colorScheme.onSurfaceVariant,
                ),
              ),
            ],
            const SizedBox(height: 12),
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                if (plugin.isRunning && onStop != null)
                  TextButton.icon(
                    onPressed: isExecuting ? null : onStop,
                    icon: const Icon(Icons.stop, size: 18),
                    label: const Text('Stop'),
                    style: TextButton.styleFrom(foregroundColor: Colors.red),
                  ),
                if (!plugin.isRunning && !plugin.isStopped && onStart != null)
                  TextButton.icon(
                    onPressed: isExecuting ? null : onStart,
                    icon: const Icon(Icons.play_arrow, size: 18),
                    label: const Text('Start'),
                    style: TextButton.styleFrom(foregroundColor: Colors.green),
                  ),
                if (onRestart != null)
                  TextButton.icon(
                    onPressed: isExecuting || !plugin.enabled ? null : onRestart,
                    icon: const Icon(Icons.refresh, size: 18),
                    label: const Text('Restart'),
                  ),
                TextButton.icon(
                  onPressed: () {
                    // TODO: Open settings
                  },
                  icon: const Icon(Icons.settings, size: 18),
                  label: const Text('Settings'),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}