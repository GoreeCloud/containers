import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:goreecloud_containers/runtime_summary.dart';

void main() {
  final env = Platform.environment;
  final cli = env['GC_TEST_CLI'];
  final pin = env['GC_TEST_CLI_PIN'];
  final socket = env['GC_TEST_SOCKET'];
  test('real local Go client and private Unix summary bridge', () async {
    final source = LocalSummarySource(cli!, pin!, socket!);
    final summary = await source.load();
    expect(summary.total, 0);
    expect(summary.recoveryRequired, false);
    final wrong = LocalSummarySource(cli, 'sha256:${'0' * 64}', socket);
    await expectLater(wrong.load(), throwsFormatException);
  }, skip: cli == null || pin == null || socket == null);
}
