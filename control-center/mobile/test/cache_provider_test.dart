import 'package:flutter_test/flutter_test.dart';

void main() {
  group('Cache Entry', () {
    test('creates cache entry', () {
      final entry = CacheEntry(key: 'user:1', value: '{"name":"admin"}', ttl: 3600, size: 18);
      expect(entry.key, 'user:1');
      expect(entry.ttl, 3600);
    });

    test('checks if expired', () {
      final entry = CacheEntry(key: 'test', value: '', ttl: 60, size: 0, createdAt: DateTime.now().subtract(Duration(minutes: 2)));
      expect(entry.isExpired, isTrue);
    });

    test('checks if not expired', () {
      final entry = CacheEntry(key: 'test', value: '', ttl: 3600, size: 0, createdAt: DateTime.now());
      expect(entry.isExpired, isFalse);
    });
  });

  group('Cache Stats', () {
    test('calculates hit rate', () {
      final stats = CacheStats(hits: 850, misses: 150);
      expect(stats.hitRate, closeTo(0.85, 0.01));
    });

    test('calculates miss rate', () {
      final stats = CacheStats(hits: 850, misses: 150);
      expect(stats.missRate, closeTo(0.15, 0.01));
    });

    test('calculates total operations', () {
      final stats = CacheStats(hits: 850, misses: 150);
      expect(stats.totalOps, 1000);
    });
  });
}

class CacheEntry {
  final String key;
  final String value;
  final int ttl;
  final int size;
  final DateTime createdAt;

  CacheEntry({required this.key, required this.value, required this.ttl, required this.size, DateTime? createdAt}) : createdAt = createdAt ?? DateTime.now();

  bool get isExpired => DateTime.now().difference(createdAt).inSeconds > ttl;
}

class CacheStats {
  final int hits;
  final int misses;

  CacheStats({required this.hits, required this.misses});

  double get hitRate => hits / totalOps;
  double get missRate => misses / totalOps;
  int get totalOps => hits + misses;
}