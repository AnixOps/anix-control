import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/features/plugins/presentation/providers/plugins_provider.dart';

void main() {
  group('PluginsProvider', () {
    test('initial state is correct', () {
      final container = ProviderContainer();
      final state = container.read(pluginsProvider);

      expect(state.plugins, isEmpty);
      expect(state.loading, false);
      expect(state.error, isNull);
      expect(state.search, isEmpty);
      expect(state.statusFilter, isEmpty);
      expect(state.executing, false);
    });

    group('Plugin model', () {
      test('is created correctly from JSON', () {
        final json = {
          'name': 'v2ray-plugin',
          'display_name': 'V2Ray Plugin',
          'version': '1.2.3',
          'status': 'running',
          'description': 'V2Ray proxy plugin',
          'author': 'AnixOps Team',
          'enabled': true,
          'config': {'port': 443, 'network': 'tcp'},
          'last_started': '2026-03-16T10:00:00Z',
        };

        final plugin = Plugin.fromJson(json);

        expect(plugin.name, 'v2ray-plugin');
        expect(plugin.displayName, 'V2Ray Plugin');
        expect(plugin.version, '1.2.3');
        expect(plugin.status, 'running');
        expect(plugin.description, 'V2Ray proxy plugin');
        expect(plugin.author, 'AnixOps Team');
        expect(plugin.enabled, true);
        expect(plugin.config, isNotNull);
        expect(plugin.config!['port'], 443);
      });

      test('handles missing fields with defaults', () {
        final json = {'name': 'test-plugin'};
        final plugin = Plugin.fromJson(json);

        expect(plugin.name, 'test-plugin');
        expect(plugin.displayName, 'test-plugin');
        expect(plugin.version, '0.0.0');
        expect(plugin.status, 'stopped');
        expect(plugin.description, isNull);
        expect(plugin.author, isNull);
        expect(plugin.enabled, false);
        expect(plugin.config, isNull);
        expect(plugin.lastStarted, isNull);
      });

      test('converts to JSON correctly', () {
        const plugin = Plugin(
          name: 'test-plugin',
          displayName: 'Test Plugin',
          version: '1.0.0',
          status: 'running',
          description: 'A test plugin',
          author: 'Test Author',
          enabled: true,
        );

        final json = plugin.toJson();

        expect(json['name'], 'test-plugin');
        expect(json['display_name'], 'Test Plugin');
        expect(json['version'], '1.0.0');
        expect(json['status'], 'running');
        expect(json['description'], 'A test plugin');
        expect(json['author'], 'Test Author');
        expect(json['enabled'], true);
      });

      test('isRunning returns correct value', () {
        const runningPlugin = Plugin(
          name: 'test',
          displayName: 'Test',
          version: '1.0',
          status: 'running',
        );
        expect(runningPlugin.isRunning, true);
        expect(runningPlugin.isStopped, false);
        expect(runningPlugin.isError, false);
      });

      test('isStopped returns correct value', () {
        const stoppedPlugin = Plugin(
          name: 'test',
          displayName: 'Test',
          version: '1.0',
          status: 'stopped',
        );
        expect(stoppedPlugin.isRunning, false);
        expect(stoppedPlugin.isStopped, true);
        expect(stoppedPlugin.isError, false);
      });

      test('isError returns correct value', () {
        const errorPlugin = Plugin(
          name: 'test',
          displayName: 'Test',
          version: '1.0',
          status: 'error',
        );
        expect(errorPlugin.isRunning, false);
        expect(errorPlugin.isStopped, false);
        expect(errorPlugin.isError, true);
      });
    });

    group('PluginsState', () {
      test('copyWith works correctly', () {
        const original = PluginsState();
        final updated = original.copyWith(
          loading: true,
          error: 'Test error',
          search: 'test',
        );

        expect(updated.loading, true);
        expect(updated.error, 'Test error');
        expect(updated.search, 'test');
        expect(updated.plugins, isEmpty);
      });

      test('filteredPlugins filters by search', () {
        final state = PluginsState(
          plugins: [
            const Plugin(name: 'v2ray', displayName: 'V2Ray Plugin', version: '1.0'),
            const Plugin(name: 'trojan', displayName: 'Trojan Plugin', version: '1.0'),
            const Plugin(name: 'shadowsocks', displayName: 'Shadowsocks', version: '1.0'),
          ],
          search: 'v2ray',
        );

        expect(state.filteredPlugins.length, 1);
        expect(state.filteredPlugins.first.name, 'v2ray');
      });

      test('filteredPlugins filters by status', () {
        final state = PluginsState(
          plugins: [
            const Plugin(name: 'plugin1', displayName: 'Plugin 1', version: '1.0', status: 'running'),
            const Plugin(name: 'plugin2', displayName: 'Plugin 2', version: '1.0', status: 'stopped'),
            const Plugin(name: 'plugin3', displayName: 'Plugin 3', version: '1.0', status: 'running'),
          ],
          statusFilter: 'running',
        );

        expect(state.filteredPlugins.length, 2);
      });

      test('filteredPlugins combines search and status filters', () {
        final state = PluginsState(
          plugins: [
            const Plugin(name: 'v2ray-running', displayName: 'V2Ray Running', version: '1.0', status: 'running'),
            const Plugin(name: 'v2ray-stopped', displayName: 'V2Ray Stopped', version: '1.0', status: 'stopped'),
            const Plugin(name: 'trojan', displayName: 'Trojan', version: '1.0', status: 'running'),
          ],
          search: 'v2ray',
          statusFilter: 'running',
        );

        expect(state.filteredPlugins.length, 1);
        expect(state.filteredPlugins.first.name, 'v2ray-running');
      });

      test('runningCount computes correctly', () {
        final state = PluginsState(
          plugins: [
            const Plugin(name: 'p1', displayName: 'P1', version: '1.0', status: 'running'),
            const Plugin(name: 'p2', displayName: 'P2', version: '1.0', status: 'stopped'),
            const Plugin(name: 'p3', displayName: 'P3', version: '1.0', status: 'running'),
          ],
        );

        expect(state.runningCount, 2);
      });

      test('stoppedCount computes correctly', () {
        final state = PluginsState(
          plugins: [
            const Plugin(name: 'p1', displayName: 'P1', version: '1.0', status: 'running'),
            const Plugin(name: 'p2', displayName: 'P2', version: '1.0', status: 'stopped'),
            const Plugin(name: 'p3', displayName: 'P3', version: '1.0', status: 'stopped'),
          ],
        );

        expect(state.stoppedCount, 2);
      });

      test('enabledCount computes correctly', () {
        final state = PluginsState(
          plugins: [
            const Plugin(name: 'p1', displayName: 'P1', version: '1.0', enabled: true),
            const Plugin(name: 'p2', displayName: 'P2', version: '1.0', enabled: false),
            const Plugin(name: 'p3', displayName: 'P3', version: '1.0', enabled: true),
          ],
        );

        expect(state.enabledCount, 2);
      });
    });

    group('pluginProvider family', () {
      test('returns plugin by name', () {
        final container = ProviderContainer();
        final notifier = container.read(pluginsProvider.notifier);

        // Set state with plugins
        notifier.state = PluginsState(
          plugins: [
            const Plugin(name: 'v2ray', displayName: 'V2Ray', version: '1.0'),
            const Plugin(name: 'trojan', displayName: 'Trojan', version: '1.0'),
          ],
        );

        final v2rayPlugin = container.read(pluginProvider('v2ray'));
        expect(v2rayPlugin, isNotNull);
        expect(v2rayPlugin!.name, 'v2ray');

        final notFoundPlugin = container.read(pluginProvider('nonexistent'));
        expect(notFoundPlugin, isNull);
      });
    });
  });
}