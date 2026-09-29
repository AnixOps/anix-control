import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/features/users/presentation/providers/users_provider.dart';

void main() {
  group('UsersProvider', () {
    test('initial state is correct', () {
      final container = ProviderContainer();
      final state = container.read(usersProvider);

      expect(state.users, isEmpty);
      expect(state.loading, false);
      expect(state.error, isNull);
      expect(state.search, isEmpty);
    });

    test('User model is created correctly from JSON', () {
      final json = {
        'id': '1',
        'email': 'test@example.com',
        'name': 'Test User',
        'role': 'admin',
        'status': 'active',
        'traffic_used': 1000000000,
        'traffic_limit': 10000000000,
      };

      final user = User.fromJson(json);

      expect(user.id, '1');
      expect(user.email, 'test@example.com');
      expect(user.name, 'Test User');
      expect(user.role, 'admin');
      expect(user.status, 'active');
      expect(user.trafficUsed, 1000000000);
      expect(user.trafficLimit, 10000000000);
    });

    test('User isAdmin returns true for admin role', () {
      const user = User(
        id: '1',
        email: 'admin@example.com',
        role: 'admin',
      );

      expect(user.isAdmin, true);
    });

    test('User isAdmin returns false for non-admin role', () {
      const user = User(
        id: '1',
        email: 'user@example.com',
        role: 'user',
      );

      expect(user.isAdmin, false);
    });

    test('User isActive returns true for active status', () {
      const user = User(
        id: '1',
        email: 'user@example.com',
        status: 'active',
      );

      expect(user.isActive, true);
    });

    test('User isBanned returns true for banned status', () {
      const user = User(
        id: '1',
        email: 'user@example.com',
        status: 'banned',
      );

      expect(user.isBanned, true);
    });

    test('User trafficUsagePercent calculates correctly', () {
      const user = User(
        id: '1',
        email: 'user@example.com',
        trafficUsed: 5000000000,
        trafficLimit: 10000000000,
      );

      expect(user.trafficUsagePercent, 50.0);
    });

    test('User trafficUsagePercent returns null when trafficLimit is null', () {
      const user = User(
        id: '1',
        email: 'user@example.com',
        trafficUsed: 5000000000,
      );

      expect(user.trafficUsagePercent, isNull);
    });

    test('UsersState computes filteredUsers correctly', () {
      const state = UsersState(
        users: [
          User(id: '1', email: 'user1@example.com', name: 'User One'),
          User(id: '2', email: 'admin@example.com', name: 'Admin User'),
          User(id: '3', email: 'user2@example.com', name: 'User Two'),
        ],
        search: 'admin',
      );

      expect(state.filteredUsers.length, 1);
      expect(state.filteredUsers.first.email, 'admin@example.com');
    });

    test('UsersState computes activeCount correctly', () {
      const state = UsersState(
        users: [
          User(id: '1', email: 'user1@example.com', status: 'active'),
          User(id: '2', email: 'user2@example.com', status: 'banned'),
          User(id: '3', email: 'user3@example.com', status: 'active'),
        ],
      );

      expect(state.activeCount, 2);
    });

    test('UsersState computes bannedCount correctly', () {
      const state = UsersState(
        users: [
          User(id: '1', email: 'user1@example.com', status: 'active'),
          User(id: '2', email: 'user2@example.com', status: 'banned'),
          User(id: '3', email: 'user3@example.com', status: 'banned'),
        ],
      );

      expect(state.bannedCount, 2);
    });

    test('UsersState computes adminCount correctly', () {
      const state = UsersState(
        users: [
          User(id: '1', email: 'user1@example.com', role: 'user'),
          User(id: '2', email: 'admin@example.com', role: 'admin'),
          User(id: '3', email: 'user3@example.com', role: 'user'),
        ],
      );

      expect(state.adminCount, 1);
    });
  });
}