import 'package:flutter_test/flutter_test.dart';
import 'package:anixops_mobile/core/models/playbook_models.dart';
import 'package:anixops_mobile/features/playbooks/presentation/providers/playbooks_provider.dart';

void main() {
  group('Playbook model', () {
    test('is created correctly from JSON', () {
      final json = {
        'id': 1,
        'name': 'install-docker',
        'storage_key': 'playbooks/install-docker.yml',
        'description': 'Install Docker on target nodes',
        'category': 'software',
        'source': 'built-in',
        'variables': '{"version": {"default": "latest"}}',
        'tags': 'docker,container',
        'author': 'anixops',
        'version': '1.0.0',
        'created_at': '2026-03-20T10:00:00Z',
        'updated_at': '2026-03-20T12:00:00Z',
      };

      final playbook = Playbook.fromJson(json);

      expect(playbook.id, 1);
      expect(playbook.name, 'install-docker');
      expect(playbook.storageKey, 'playbooks/install-docker.yml');
      expect(playbook.description, 'Install Docker on target nodes');
      expect(playbook.category, 'software');
      expect(playbook.source, 'built-in');
      expect(playbook.variables, isNotNull);
      expect(playbook.tags, 'docker,container');
      expect(playbook.author, 'anixops');
      expect(playbook.version, '1.0.0');
    });

    test('handles missing optional fields', () {
      final json = {
        'id': 2,
        'name': 'minimal-playbook',
        'storage_key': 'playbooks/minimal.yml',
        'created_at': '2026-03-20T10:00:00Z',
        'updated_at': '2026-03-20T12:00:00Z',
      };

      final playbook = Playbook.fromJson(json);

      expect(playbook.id, 2);
      expect(playbook.name, 'minimal-playbook');
      expect(playbook.storageKey, 'playbooks/minimal.yml');
      expect(playbook.description, isNull);
      expect(playbook.category, isNull);
      expect(playbook.variables, isNull);
      expect(playbook.tags, isNull);
    });
  });

  group('PlaybooksState', () {
    test('filteredPlaybooks returns all when no category selected', () {
      final state = PlaybooksState(
        playbooks: [
          Playbook(id: 1, name: 'playbook-1', storageKey: 'k1', category: 'security', createdAt: '', updatedAt: ''),
          Playbook(id: 2, name: 'playbook-2', storageKey: 'k2', category: 'maintenance', createdAt: '', updatedAt: ''),
        ],
      );

      expect(state.filteredPlaybooks.length, 2);
    });

    test('filteredPlaybooks filters by selected category', () {
      final state = PlaybooksState(
        playbooks: [
          Playbook(id: 1, name: 'playbook-1', storageKey: 'k1', category: 'security', createdAt: '', updatedAt: ''),
          Playbook(id: 2, name: 'playbook-2', storageKey: 'k2', category: 'maintenance', createdAt: '', updatedAt: ''),
        ],
        selectedCategory: 'security',
      );

      expect(state.filteredPlaybooks.length, 1);
      expect(state.filteredPlaybooks.every((p) => p.category == 'security'), true);
    });

    test('filteredPlaybooks handles "all" category', () {
      final state = PlaybooksState(
        playbooks: [
          Playbook(id: 1, name: 'playbook-1', storageKey: 'k1', category: 'security', createdAt: '', updatedAt: ''),
        ],
        selectedCategory: 'all',
      );

      expect(state.filteredPlaybooks.length, 1);
    });

    test('copyWith works correctly', () {
      const state = PlaybooksState(
        playbooks: [],
        isLoading: false,
      );

      final newState = state.copyWith(
        isLoading: true,
        error: 'Test error',
        selectedCategory: 'security',
      );

      expect(newState.isLoading, true);
      expect(newState.error, 'Test error');
      expect(newState.selectedCategory, 'security');
      expect(newState.playbooks, isEmpty);
    });
  });
}