import 'package:flutter_test/flutter_test.dart';
import 'package:goreecloud_containers/main.dart';
import 'package:goreecloud_containers/runtime_summary.dart';

class Fixture implements SummarySource {
  @override
  Future<RuntimeSummary> load() async =>
      const RuntimeSummary(3, 2, 0, 0, 1, true);
}

class Broken implements SummarySource {
  @override
  Future<RuntimeSummary> load() async => throw StateError('private error');
}

class Counting implements SummarySource {
  int calls = 0;
  final int incidentAt;
  Counting({this.incidentAt = 100});
  @override
  Future<RuntimeSummary> load() async {
    calls++;
    return calls >= incidentAt
        ? const RuntimeSummary(1, 0, 0, 0, 1, true)
        : const RuntimeSummary(1, 1, 0, 0, 0, false);
  }
}

void main() {
  testWidgets('explicit monitoring stops on an incident', (tester) async {
    final source = Counting(incidentAt: 2);
    await tester.pumpWidget(ContainersApp(source: source));
    expect(source.calls, 0);
    await tester.tap(find.byTooltip('Start monitoring'));
    await tester.pump();
    await tester.pumpAndSettle();
    expect(source.calls, 1);
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(source.calls, 2);
    expect(find.textContaining('Monitoring stopped: review'), findsOneWidget);
    await tester.pump(const Duration(seconds: 10));
    expect(source.calls, 2);
  });
  testWidgets('explicit monitoring can be stopped and is bounded',
      (tester) async {
    final source = Counting();
    await tester.pumpWidget(ContainersApp(source: source));
    await tester.tap(find.byTooltip('Start monitoring'));
    await tester.pumpAndSettle();
    await tester.tap(find.byTooltip('Stop monitoring'));
    await tester.pumpAndSettle();
    await tester.pump(const Duration(seconds: 10));
    expect(source.calls, 1);
    await tester.tap(find.byTooltip('Start monitoring'));
    await tester.pumpAndSettle();
    for (var i = 0; i < 9; i++) {
      await tester.pump(const Duration(seconds: 2));
      await tester.pumpAndSettle();
    }
    expect(source.calls, 11);
    expect(find.text('Monitoring complete: 10 observations.'), findsOneWidget);
    await tester.pump(const Duration(seconds: 10));
    expect(source.calls, 11);
  });

  testWidgets('disconnected state and safety page', (tester) async {
    await tester.pumpWidget(const ContainersApp());
    expect(find.text('Unavailable'), findsNWidgets(5));
    await tester.tap(find.text('Safety'));
    await tester.pumpAndSettle();
    expect(find.text('Trust boundaries'), findsOneWidget);
  });
  testWidgets('refresh displays counts and recovery warning', (tester) async {
    await tester.pumpWidget(ContainersApp(source: Fixture()));
    await tester.tap(find.byTooltip('Refresh runtime summary'));
    await tester.pumpAndSettle();
    expect(find.text('Recovery review required. No automatic repair.'),
        findsOneWidget);
    expect(find.text('2'), findsOneWidget);
  });
  testWidgets('failure stays unavailable and suppresses raw error',
      (tester) async {
    await tester.pumpWidget(ContainersApp(source: Broken()));
    await tester.tap(find.byTooltip('Refresh runtime summary'));
    await tester.pumpAndSettle();
    expect(find.text('Unavailable'), findsNWidgets(5));
    expect(find.textContaining('private error'), findsNothing);
    expect(find.textContaining('Local runtime unavailable'), findsOneWidget);
  });
  test('rejects incorrect policy and counts', () {
    const valid =
        '{"readOnly":true,"total":2,"running":1,"created":0,"stopped":0,"uncertain":1,"recoveryRequired":true,"atomicSnapshot":false,"identityIntegrationVerified":false,"deploymentAuthorized":false}';
    expect(RuntimeSummary.parse(valid, 1).uncertain, 1);
    expect(() => RuntimeSummary.parse(valid, 0), throwsFormatException);
    expect(
        () => RuntimeSummary.parse(
            valid.replaceFirst('"total":2', '"total":3'), 1),
        throwsFormatException);
    expect(
        () => RuntimeSummary.parse(
            valid.replaceFirst(
                '"deploymentAuthorized":false', '"deploymentAuthorized":true'),
            1),
        throwsFormatException);
  });
}
