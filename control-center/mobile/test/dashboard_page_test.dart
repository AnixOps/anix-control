import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:anixops_mobile/features/dashboard/presentation/providers/dashboard_provider.dart';
import 'package:anixops_mobile/features/dashboard/presentation/pages/dashboard_page.dart';

void main() {
  group('DashboardPage', () {
    testWidgets('renders correctly', (WidgetTester tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            home: DashboardPage(),
          ),
        ),
      );

      // Pump a few times to allow microtasks
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 100));

      // Verify app bar is rendered
      expect(find.text('Dashboard'), findsOneWidget);

      // Verify refresh indicator is present
      expect(find.byType(RefreshIndicator), findsOneWidget);
    });

    testWidgets('shows overview section', (WidgetTester tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            home: DashboardPage(),
          ),
        ),
      );

      await tester.pump();
      await tester.pump(const Duration(milliseconds: 100));

      // Verify overview title
      expect(find.text('Overview'), findsOneWidget);

      // Verify stat cards exist
      expect(find.text('Nodes'), findsOneWidget);
      expect(find.text('Users'), findsOneWidget);
    });

    testWidgets('shows quick actions', (WidgetTester tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            home: DashboardPage(),
          ),
        ),
      );

      await tester.pump();
      await tester.pump(const Duration(milliseconds: 100));

      // Verify quick actions section
      expect(find.text('Quick Actions'), findsOneWidget);
      expect(find.text('Add Node'), findsOneWidget);
      expect(find.text('Playbooks'), findsOneWidget);
      expect(find.text('Tasks'), findsOneWidget);
      expect(find.text('Schedules'), findsOneWidget);
    });

    testWidgets('shows system health section', (WidgetTester tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            home: DashboardPage(),
          ),
        ),
      );

      await tester.pump();
      await tester.pump(const Duration(milliseconds: 100));

      // Verify system health section
      expect(find.text('System Health'), findsOneWidget);
      expect(find.text('CPU'), findsOneWidget);
      expect(find.text('Memory'), findsOneWidget);
      expect(find.text('Disk'), findsOneWidget);
    });
  });
}