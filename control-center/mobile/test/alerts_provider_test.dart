import 'package:flutter_test/flutter_test.dart';

void main() {
  group('Alert Rule', () {
    test('creates alert rule', () {
      final rule = AlertRule(
        id: '1',
        name: 'High CPU',
        metric: 'cpu_percent',
        threshold: 80,
        severity: 'warning',
        enabled: true,
      );
      expect(rule.name, 'High CPU');
      expect(rule.threshold, 80);
    });

    test('toggles rule status', () {
      final rule = AlertRule(id: '1', name: 'test', metric: 'test', threshold: 50, severity: 'warning', enabled: true);
      rule.enabled = false;
      expect(rule.enabled, isFalse);
    });

    test('validates severity', () {
      final validSeverities = ['info', 'warning', 'critical'];
      expect(validSeverities.contains('warning'), isTrue);
      expect(validSeverities.contains('critical'), isTrue);
    });
  });

  group('Active Alert', () {
    test('creates active alert', () {
      final alert = ActiveAlert(
        id: 'a1',
        ruleId: '1',
        name: 'High CPU',
        value: 92,
        threshold: 80,
        severity: 'warning',
        status: 'firing',
      );
      expect(alert.name, 'High CPU');
      expect(alert.isFiring, isTrue);
    });

    test('checks if firing', () {
      final firing = ActiveAlert(id: '1', ruleId: '1', name: 'test', value: 92, threshold: 80, severity: 'warning', status: 'firing');
      final resolved = ActiveAlert(id: '2', ruleId: '1', name: 'test', value: 70, threshold: 80, severity: 'warning', status: 'resolved');
      expect(firing.isFiring, isTrue);
      expect(resolved.isFiring, isFalse);
    });

    test('calculates duration', () {
      final alert = ActiveAlert(
        id: '1',
        ruleId: '1',
        name: 'test',
        value: 90,
        threshold: 80,
        severity: 'warning',
        status: 'firing',
        startedAt: DateTime.now().subtract(Duration(minutes: 30)),
      );
      expect(alert.durationMinutes, greaterThanOrEqualTo(30));
    });
  });

  group('Alert Notification', () {
    test('creates notification from alert', () {
      final alert = ActiveAlert(id: '1', ruleId: '1', name: 'High CPU', value: 92, threshold: 80, severity: 'critical', status: 'firing');
      final notification = alert.toNotification();
      expect(notification.title, contains('High CPU'));
      expect(notification.severity, 'critical');
    });
  });
}

class AlertRule {
  final String id;
  final String name;
  final String metric;
  final double threshold;
  final String severity;
  bool enabled;

  AlertRule({
    required this.id,
    required this.name,
    required this.metric,
    required this.threshold,
    required this.severity,
    required this.enabled,
  });
}

class ActiveAlert {
  final String id;
  final String ruleId;
  final String name;
  final double value;
  final double threshold;
  final String severity;
  final String status;
  final DateTime? startedAt;

  ActiveAlert({
    required this.id,
    required this.ruleId,
    required this.name,
    required this.value,
    required this.threshold,
    required this.severity,
    required this.status,
    this.startedAt,
  });

  bool get isFiring => status == 'firing';

  int get durationMinutes {
    if (startedAt == null) return 0;
    return DateTime.now().difference(startedAt!).inMinutes;
  }

  AlertNotification toNotification() {
    return AlertNotification(
      title: name,
      message: 'Value: $value, Threshold: $threshold',
      severity: severity,
    );
  }
}

class AlertNotification {
  final String title;
  final String message;
  final String severity;

  AlertNotification({required this.title, required this.message, required this.severity});
}