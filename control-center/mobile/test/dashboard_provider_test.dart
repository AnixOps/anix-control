import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/features/dashboard/presentation/providers/dashboard_provider.dart';

void main() {
  group('DashboardProvider', () {
    test('initial state is correct', () {
      final container = ProviderContainer();
      final state = container.read(dashboardProvider);

      expect(state.stats.nodes.total, 0);
      expect(state.stats.nodes.online, 0);
      expect(state.stats.nodes.offline, 0);
      expect(state.stats.users.total, 0);
      expect(state.stats.users.active, 0);
      expect(state.stats.agents.total, 0);
      expect(state.stats.agents.online, 0);
      expect(state.stats.traffic.today, 0);
      expect(state.stats.traffic.month, 0);
      expect(state.stats.plugins.total, 0);
      expect(state.stats.plugins.active, 0);
      expect(state.activities, isEmpty);
      expect(state.alerts, isEmpty);
      expect(state.loading, false);
      expect(state.error, isNull);
      expect(state.lastUpdate, isNull);
    });

    group('DashboardStats', () {
      test('is created correctly from JSON', () {
        final json = {
          'nodes': {'total': 10, 'online': 8, 'offline': 2},
          'users': {'total': 100, 'active': 75},
          'agents': {'total': 5, 'online': 4},
          'traffic': {'today': 1000000000, 'month': 30000000000},
          'plugins': {'total': 20, 'active': 15},
        };

        final stats = DashboardStats.fromJson(json);

        expect(stats.nodes.total, 10);
        expect(stats.nodes.online, 8);
        expect(stats.nodes.offline, 2);
        expect(stats.users.total, 100);
        expect(stats.users.active, 75);
        expect(stats.agents.total, 5);
        expect(stats.agents.online, 4);
        expect(stats.traffic.today, 1000000000);
        expect(stats.traffic.month, 30000000000);
        expect(stats.plugins.total, 20);
        expect(stats.plugins.active, 15);
      });

      test('handles missing fields with defaults', () {
        final json = <String, dynamic>{};
        final stats = DashboardStats.fromJson(json);

        expect(stats.nodes.total, 0);
        expect(stats.users.total, 0);
        expect(stats.agents.total, 0);
        expect(stats.traffic.today, 0);
        expect(stats.plugins.total, 0);
      });
    });

    group('NodeStats', () {
      test('is created correctly from JSON', () {
        final json = {'total': 24, 'online': 20, 'offline': 4};
        final stats = NodeStats.fromJson(json);

        expect(stats.total, 24);
        expect(stats.online, 20);
        expect(stats.offline, 4);
      });

      test('handles partial JSON', () {
        final json = {'total': 10};
        final stats = NodeStats.fromJson(json);

        expect(stats.total, 10);
        expect(stats.online, 0);
        expect(stats.offline, 0);
      });
    });

    group('UserStats', () {
      test('is created correctly from JSON', () {
        final json = {'total': 500, 'active': 450};
        final stats = UserStats.fromJson(json);

        expect(stats.total, 500);
        expect(stats.active, 450);
      });
    });

    group('AgentStats', () {
      test('is created correctly from JSON', () {
        final json = {'total': 15, 'online': 12};
        final stats = AgentStats.fromJson(json);

        expect(stats.total, 15);
        expect(stats.online, 12);
      });
    });

    group('TrafficStats', () {
      test('is created correctly from JSON', () {
        final json = {'today': 5000000000, 'month': 150000000000};
        final stats = TrafficStats.fromJson(json);

        expect(stats.today, 5000000000);
        expect(stats.month, 150000000000);
      });
    });

    group('PluginStats', () {
      test('is created correctly from JSON', () {
        final json = {'total': 10, 'active': 8};
        final stats = PluginStats.fromJson(json);

        expect(stats.total, 10);
        expect(stats.active, 8);
      });
    });

    group('Activity', () {
      test('is created correctly from JSON', () {
        final json = {
          'id': 1,
          'type': 'node_deployed',
          'message': 'Node tokyo-01 deployed successfully',
          'user_id': 5,
          'user_name': 'admin',
          'timestamp': '2026-03-16T10:30:00Z',
          'metadata': {'node_id': 'node-123'},
        };

        final activity = Activity.fromJson(json);

        expect(activity.id, '1');
        expect(activity.type, 'node_deployed');
        expect(activity.message, 'Node tokyo-01 deployed successfully');
        expect(activity.userId, '5');
        expect(activity.userName, 'admin');
        expect(activity.metadata, isNotNull);
        expect(activity.metadata!['node_id'], 'node-123');
      });

      test('handles missing optional fields', () {
        final json = {
          'id': 2,
          'type': 'system',
          'message': 'System started',
        };

        final activity = Activity.fromJson(json);

        expect(activity.id, '2');
        expect(activity.type, 'system');
        expect(activity.message, 'System started');
        expect(activity.userId, isNull);
        expect(activity.userName, isNull);
        expect(activity.metadata, isNull);
      });
    });

    group('Alert', () {
      test('is created correctly from JSON', () {
        final json = {
          'id': 'alert-1',
          'level': 'critical',
          'title': 'High CPU Usage',
          'message': 'Node tokyo-01 CPU usage is at 95%',
          'timestamp': '2026-03-16T12:00:00Z',
          'acknowledged': false,
        };

        final alert = Alert.fromJson(json);

        expect(alert.id, 'alert-1');
        expect(alert.level, 'critical');
        expect(alert.title, 'High CPU Usage');
        expect(alert.message, 'Node tokyo-01 CPU usage is at 95%');
        expect(alert.acknowledged, false);
      });

      test('defaults acknowledged to false', () {
        final json = {
          'id': 'alert-2',
          'level': 'warning',
          'title': 'Test Alert',
          'message': 'Test message',
          'timestamp': '2026-03-16T12:00:00Z',
        };

        final alert = Alert.fromJson(json);

        expect(alert.acknowledged, false);
      });
    });

    group('DashboardState', () {
      test('copyWith works correctly', () {
        const original = DashboardState();
        final updated = original.copyWith(
          loading: true,
          error: 'Test error',
        );

        expect(updated.loading, true);
        expect(updated.error, 'Test error');
        expect(updated.stats.nodes.total, 0); // Unchanged
      });

      test('hasAlerts computes correctly', () {
        const state1 = DashboardState();
        expect(state1.hasAlerts, false);

        final state2 = DashboardState(
          alerts: [
            Alert(
              id: '1',
              level: 'warning',
              title: 'Test',
              message: 'Test',
              timestamp: DateTime.now(),
            ),
          ],
        );
        expect(state2.hasAlerts, true);
      });

      test('criticalAlerts filters correctly', () {
        final state = DashboardState(
          alerts: [
            Alert(
              id: '1',
              level: 'critical',
              title: 'Critical 1',
              message: 'Message 1',
              timestamp: DateTime.now(),
            ),
            Alert(
              id: '2',
              level: 'warning',
              title: 'Warning 1',
              message: 'Message 2',
              timestamp: DateTime.now(),
            ),
            Alert(
              id: '3',
              level: 'critical',
              title: 'Critical 2',
              message: 'Message 3',
              timestamp: DateTime.now(),
            ),
          ],
        );

        expect(state.criticalAlerts.length, 2);
        expect(state.warningAlerts.length, 1);
      });
    });
  });
}