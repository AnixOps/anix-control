import 'package:flutter_test/flutter_test.dart';

void main() {
  group('Health Check', () {
    test('creates health check', () {
      final check = HealthCheck(name: 'API', status: 'healthy', latency: 12);
      expect(check.name, 'API');
      expect(check.status, 'healthy');
      expect(check.latency, 12);
    });

    test('checks if healthy', () {
      final healthy = HealthCheck(name: 'test', status: 'healthy', latency: 10);
      final degraded = HealthCheck(name: 'test', status: 'degraded', latency: 50);
      expect(healthy.isHealthy, isTrue);
      expect(degraded.isHealthy, isFalse);
    });

    test('formats latency', () {
      final check = HealthCheck(name: 'test', status: 'healthy', latency: 125);
      expect(check.formattedLatency, '125ms');
    });
  });

  group('Overall Health', () {
    test('returns healthy if all healthy', () {
      final checks = [
        HealthCheck(name: 'a', status: 'healthy', latency: 10),
        HealthCheck(name: 'b', status: 'healthy', latency: 10),
      ];
      final overall = getOverallHealth(checks);
      expect(overall, 'healthy');
    });

    test('returns degraded if any degraded', () {
      final checks = [
        HealthCheck(name: 'a', status: 'healthy', latency: 10),
        HealthCheck(name: 'b', status: 'degraded', latency: 50),
      ];
      final overall = getOverallHealth(checks);
      expect(overall, 'degraded');
    });

    test('returns unhealthy if any unhealthy', () {
      final checks = [
        HealthCheck(name: 'a', status: 'healthy', latency: 10),
        HealthCheck(name: 'b', status: 'unhealthy', latency: 0),
      ];
      final overall = getOverallHealth(checks);
      expect(overall, 'unhealthy');
    });
  });

  group('Dependency', () {
    test('creates dependency', () {
      final dep = Dependency(name: 'PostgreSQL', type: 'database', status: 'healthy', version: '15.2');
      expect(dep.name, 'PostgreSQL');
      expect(dep.type, 'database');
    });

    test('groups by type', () {
      final deps = [
        Dependency(name: 'PostgreSQL', type: 'database', status: 'healthy', version: '15'),
        Dependency(name: 'Redis', type: 'cache', status: 'healthy', version: '7'),
      ];
      final types = deps.map((d) => d.type).toSet();
      expect(types.length, 2);
    });
  });
}

String getOverallHealth(List<HealthCheck> checks) {
  if (checks.any((c) => c.status == 'unhealthy')) return 'unhealthy';
  if (checks.any((c) => c.status == 'degraded')) return 'degraded';
  return 'healthy';
}

class HealthCheck {
  final String name;
  final String status;
  final int latency;

  HealthCheck({required this.name, required this.status, required this.latency});

  bool get isHealthy => status == 'healthy';
  String get formattedLatency => '${latency}ms';
}

class Dependency {
  final String name;
  final String type;
  final String status;
  final String version;

  Dependency({required this.name, required this.type, required this.status, required this.version});
}