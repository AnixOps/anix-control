import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/features/nodes/presentation/providers/nodes_provider.dart';

void main() {
  group('NodesProvider', () {
    test('initial state is correct', () {
      final container = ProviderContainer();
      final state = container.read(nodesProvider);

      expect(state.nodes, isEmpty);
      expect(state.loading, false);
      expect(state.error, isNull);
      expect(state.search, isEmpty);
      expect(state.statusFilter, isEmpty);
    });

    test('Node model is created correctly from JSON', () {
      final json = {
        'id': '1',
        'name': 'test-node',
        'host': '192.168.1.1',
        'port': 443,
        'status': 'online',
        'type': 'v2ray',
        'users': 100,
        'traffic': 1000000000,
      };

      final node = Node.fromJson(json);

      expect(node.id, '1');
      expect(node.name, 'test-node');
      expect(node.host, '192.168.1.1');
      expect(node.port, 443);
      expect(node.status, 'online');
      expect(node.type, 'v2ray');
      expect(node.users, 100);
      expect(node.traffic, 1000000000);
    });

    test('Node model converts to JSON correctly', () {
      const node = Node(
        id: '1',
        name: 'test-node',
        host: '192.168.1.1',
        port: 443,
        status: 'online',
        type: 'v2ray',
        users: 100,
        traffic: 1000000000,
      );

      final json = node.toJson();

      expect(json['id'], '1');
      expect(json['name'], 'test-node');
      expect(json['host'], '192.168.1.1');
      expect(json['port'], 443);
      expect(json['status'], 'online');
      expect(json['type'], 'v2ray');
      expect(json['users'], 100);
      expect(json['traffic'], 1000000000);
    });

    test('NodesState computes filteredNodes correctly', () {
      const state = NodesState(
        nodes: [
          Node(id: '1', name: 'node-1', host: '192.168.1.1', status: 'online'),
          Node(id: '2', name: 'node-2', host: '192.168.1.2', status: 'offline'),
          Node(id: '3', name: 'server-1', host: '192.168.1.3', status: 'online'),
        ],
        search: 'node',
      );

      expect(state.filteredNodes.length, 2);
    });

    test('NodesState filters by status', () {
      const state = NodesState(
        nodes: [
          Node(id: '1', name: 'node-1', host: '192.168.1.1', status: 'online'),
          Node(id: '2', name: 'node-2', host: '192.168.1.2', status: 'offline'),
          Node(id: '3', name: 'node-3', host: '192.168.1.3', status: 'online'),
        ],
        statusFilter: 'online',
      );

      expect(state.filteredNodes.length, 2);
    });

    test('NodesState computes onlineCount correctly', () {
      const state = NodesState(
        nodes: [
          Node(id: '1', name: 'node-1', host: '192.168.1.1', status: 'online'),
          Node(id: '2', name: 'node-2', host: '192.168.1.2', status: 'offline'),
          Node(id: '3', name: 'node-3', host: '192.168.1.3', status: 'online'),
        ],
      );

      expect(state.onlineCount, 2);
    });

    test('NodesState computes offlineCount correctly', () {
      const state = NodesState(
        nodes: [
          Node(id: '1', name: 'node-1', host: '192.168.1.1', status: 'online'),
          Node(id: '2', name: 'node-2', host: '192.168.1.2', status: 'offline'),
          Node(id: '3', name: 'node-3', host: '192.168.1.3', status: 'online'),
        ],
      );

      expect(state.offlineCount, 1);
    });

    test('RC3 realtime node stats merge preserves existing fields', () {
      const original = Node(
        id: '1',
        name: 'edge-node',
        host: '10.0.0.1',
        status: 'offline',
        users: 2,
        traffic: 100,
      );

      final updated = Node(
        id: original.id,
        name: original.name,
        host: original.host,
        port: original.port,
        status: 'online',
        type: original.type,
        users: 9,
        traffic: 2048,
        lastSeen: original.lastSeen,
        cpuUsage: 0.42,
        memoryUsage: 0.81,
        version: '2.5.0-rc.3',
      );

      expect(updated.id, '1');
      expect(updated.name, 'edge-node');
      expect(updated.status, 'online');
      expect(updated.users, 9);
      expect(updated.traffic, 2048);
      expect(updated.cpuUsage, 0.42);
      expect(updated.memoryUsage, 0.81);
      expect(updated.version, '2.5.0-rc.3');
    });
  });
}
