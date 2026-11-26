import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:crypto/crypto.dart';

class RuntimeSummary {
  final int total, running, created, stopped, uncertain;
  final bool recoveryRequired;
  const RuntimeSummary(this.total, this.running, this.created, this.stopped,
      this.uncertain, this.recoveryRequired);
  factory RuntimeSummary.parse(String raw, int exitCode) {
    final Object? value = jsonDecode(raw);
    const keys = {
      'readOnly',
      'total',
      'running',
      'created',
      'stopped',
      'uncertain',
      'recoveryRequired',
      'atomicSnapshot',
      'identityIntegrationVerified',
      'deploymentAuthorized'
    };
    if (value is! Map<String, dynamic> ||
        value.length != keys.length ||
        !keys.every(value.containsKey) ||
        value['readOnly'] != true ||
        value['atomicSnapshot'] != false ||
        value['identityIntegrationVerified'] != false ||
        value['deploymentAuthorized'] != false) {
      throw const FormatException('Unexpected runtime summary policy');
    }
    int count(String k) {
      final n = value[k];
      if (n is! int || n < 0 || n > 128) {
        throw const FormatException('Invalid runtime summary counter');
      }
      return n;
    }

    final t = count('total'),
        r = count('running'),
        c = count('created'),
        s = count('stopped'),
        u = count('uncertain');
    if (t != r + c + s + u ||
        value['recoveryRequired'] != (u > 0) ||
        (u == 0 ? exitCode != 0 : exitCode != 1)) {
      throw const FormatException('Inconsistent runtime summary');
    }
    return RuntimeSummary(t, r, c, s, u, u > 0);
  }
}

abstract interface class SummarySource {
  Future<RuntimeSummary> load();
}

/// The UI never talks to a TCP service, never invokes a shell and never
/// calls a mutating command. Reuses the Go client's Unix peer and schema
/// checks; same-user filesystem races are still possible.
class LocalSummarySource implements SummarySource {
  final String cli, cliSha256, socket;
  const LocalSummarySource(this.cli, this.cliSha256, this.socket);
  @override
  Future<RuntimeSummary> load() async {
    final file = File(cli);
    if (!cli.startsWith('/') ||
        !socket.startsWith('/') ||
        !RegExp(r'^sha256:[0-9a-f]{64}$').hasMatch(cliSha256) ||
        await file.resolveSymbolicLinks() != cli ||
        await file.length() > 64 * 1024 * 1024 ||
        await file.length() < 1 ||
        'sha256:${await sha256.bind(file.openRead()).first}' != cliSha256) {
      throw const FormatException('Reviewed local client required');
    }
    final child = await Process.start(
        cli, ['local-query', socket, 'runtime-summary'],
        runInShell: false,
        includeParentEnvironment: false,
        environment: const {'LANG': 'C'});
    final output = <int>[];
    var over = false;
    final reads = child.stdout.listen((bytes) {
      if (output.length + bytes.length > 16384) {
        over = true;
        child.kill();
      } else if (!over) {
        output.addAll(bytes);
      }
    }).asFuture<void>();
    // Discard stderr instead of displaying potentially sensitive diagnostics.
    final errs = child.stderr.drain<void>();
    try {
      final code = await child.exitCode.timeout(const Duration(seconds: 8));
      await Future.wait([reads, errs]).timeout(const Duration(seconds: 2));
      if (over) throw const FormatException('Oversized summary');
      return RuntimeSummary.parse(utf8.decode(output), code);
    } on TimeoutException {
      child.kill();
      throw const FormatException('Local summary timed out');
    }
  }
}
