import 'package:flutter_test/flutter_test.dart';

void main() {
  group('Report', () {
    test('creates report', () {
      final report = Report(id: 'r1', name: 'Weekly Summary', type: 'summary', schedule: 'weekly');
      expect(report.name, 'Weekly Summary');
      expect(report.type, 'summary');
    });

    test('checks schedule type', () {
      final daily = Report(id: 'r1', name: 'test', type: 'test', schedule: 'daily');
      final weekly = Report(id: 'r2', name: 'test', type: 'test', schedule: 'weekly');
      expect(daily.isDaily, isTrue);
      expect(weekly.isWeekly, isTrue);
    });
  });

  group('Report Generation', () {
    test('creates generation', () {
      final gen = ReportGeneration(reportId: 'r1', status: 'completed', rows: 1500);
      expect(gen.status, 'completed');
      expect(gen.rows, 1500);
    });

    test('checks if completed', () {
      final completed = ReportGeneration(reportId: 'r1', status: 'completed', rows: 100);
      final pending = ReportGeneration(reportId: 'r2', status: 'pending', rows: 0);
      expect(completed.isCompleted, isTrue);
      expect(pending.isCompleted, isFalse);
    });
  });
}

class Report {
  final String id;
  final String name;
  final String type;
  final String schedule;

  Report({required this.id, required this.name, required this.type, required this.schedule});

  bool get isDaily => schedule == 'daily';
  bool get isWeekly => schedule == 'weekly';
}

class ReportGeneration {
  final String reportId;
  final String status;
  final int rows;

  ReportGeneration({required this.reportId, required this.status, required this.rows});

  bool get isCompleted => status == 'completed';
}