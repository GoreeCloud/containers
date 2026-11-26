import 'dart:async';
import 'package:flutter/material.dart';
import 'package:goreecloud_ui_expressive/goreecloud_ui_expressive.dart';
import 'runtime_summary.dart';

void main(List<String> args) {
  // No discovery, automatic startup polling, configuration persistence or remote
  // access. The operator supplies a reviewed CLI digest and socket.
  final SummarySource? source =
      args.length == 3 ? LocalSummarySource(args[0], args[1], args[2]) : null;
  runApp(ContainersApp(source: source));
}

class ContainersApp extends StatelessWidget {
  final SummarySource? source;
  const ContainersApp({super.key, this.source});
  @override
  Widget build(BuildContext context) => MaterialApp(
        title: 'GoreeCloud Containers',
        debugShowCheckedModeBanner: false,
        theme: GoreeCloudFlutterTheme.build(brightness: Brightness.light),
        darkTheme: GoreeCloudFlutterTheme.build(brightness: Brightness.dark),
        home: Dashboard(source: source),
      );
}

class Dashboard extends StatefulWidget {
  final SummarySource? source;
  const Dashboard({super.key, this.source});
  @override
  State<Dashboard> createState() => _DashboardState();
}

class _DashboardState extends State<Dashboard> with WidgetsBindingObserver {
  RuntimeSummary? summary;
  bool busy = false;
  String? error;
  DateTime? observed;
  int page = 0;
  Timer? _monitorTimer;
  bool monitoring = false;
  int monitorSamples = 0;
  String? monitorStatus;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void dispose() {
    _monitorTimer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed && monitoring) {
      stopMonitoring('Monitoring stopped when window lost foreground.');
    }
  }

  void stopMonitoring(String reason) {
    _monitorTimer?.cancel();
    _monitorTimer = null;
    if (mounted) {
      setState(() {
        monitoring = false;
        monitorStatus = reason;
      });
    }
  }

  Future<void> startMonitoring() async {
    if (busy || monitoring || widget.source == null || page != 0) return;
    setState(() {
      monitoring = true;
      monitorSamples = 0;
      monitorStatus = 'Monitoring active · at most 10 observations';
    });
    await monitorOnce();
  }

  Future<void> monitorOnce() async {
    if (!mounted || !monitoring || busy) return;
    await refresh();
    if (!mounted || !monitoring) return;
    setState(() => monitorSamples++);
    if (error != null || summary == null || summary!.recoveryRequired) {
      stopMonitoring(
          'Monitoring stopped: review runtime availability or recovery.');
    } else if (monitorSamples >= 10) {
      stopMonitoring('Monitoring complete: 10 observations.');
    } else {
      _monitorTimer = Timer(const Duration(seconds: 2), monitorOnce);
    }
  }

  void selectPage(int value) {
    if (value != 0 && monitoring) {
      stopMonitoring('Monitoring stopped when leaving overview.');
    }
    setState(() => page = value);
  }

  Future<void> refresh() async {
    if (busy || widget.source == null) return;
    setState(() {
      busy = true;
      error = null;
      summary = null;
      observed = null;
    });
    try {
      final next = await widget.source!.load();
      if (mounted) {
        setState(() {
          summary = next;
          observed = DateTime.now();
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() {
          error =
              'Local runtime unavailable or response rejected. Check the reviewed client and private socket.';
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          busy = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final s = summary;
    final items = <(String, int?, IconData)>[
      ('Total', s?.total, Icons.inventory_2_outlined),
      ('Running', s?.running, Icons.play_circle_outline),
      ('Created', s?.created, Icons.add_box_outlined),
      ('Stopped', s?.stopped, Icons.stop_circle_outlined),
      ('Uncertain', s?.uncertain, Icons.warning_amber_rounded),
    ];
    return Scaffold(
      appBar: AppBar(
        title: const Text('GoreeCloud Containers'),
        actions: [
          IconButton(
              tooltip: 'Refresh runtime summary',
              onPressed:
                  busy || monitoring || widget.source == null ? null : refresh,
              icon: const Icon(Icons.refresh)),
          IconButton(
            tooltip: monitoring ? 'Stop monitoring' : 'Start monitoring',
            onPressed: monitoring
                ? () => stopMonitoring('Monitoring stopped by operator.')
                : (busy || widget.source == null || page != 0
                    ? null
                    : startMonitoring),
            icon: Icon(monitoring
                ? Icons.stop_circle_outlined
                : Icons.monitor_heart_outlined),
          ),
          const SizedBox(width: 12),
        ],
      ),
      bottomNavigationBar: GoreeCloudFloatingNavigation(
        selectedIndex: page,
        onDestinationSelected: selectPage,
        destinations: const [
          NavigationDestination(
              icon: Icon(Icons.dashboard_outlined), label: 'Overview'),
          NavigationDestination(
              icon: Icon(Icons.verified_user_outlined), label: 'Safety'),
        ],
      ),
      body: SafeArea(
          child: Center(
              child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 1000),
        child: SingleChildScrollView(
            padding: const EdgeInsets.all(24),
            child:
                Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(page == 0 ? 'Runtime overview' : 'Trust boundaries',
                  style: theme.textTheme.headlineMedium),
              const SizedBox(height: 8),
              Text('Local · read-only · development preview',
                  style: theme.textTheme.bodyLarge),
              const SizedBox(height: 24),
              if (page == 0) ...[
                if (widget.source == null)
                  const Text(
                      'Start with a reviewed local CLI path, SHA-256 digest and private Unix socket. No runtime connected.'),
                if (monitorStatus != null)
                  Semantics(liveRegion: true, child: Text(monitorStatus!)),
                if (monitoring) Text('Observations: $monitorSamples / 10'),
                if (busy) const LinearProgressIndicator(),
                if (error != null)
                  Semantics(
                      liveRegion: true,
                      child: Text(error!,
                          style: TextStyle(color: theme.colorScheme.error))),
                if (observed != null)
                  Text('Observed locally: ${observed!.toLocal()}'),
                if (s?.recoveryRequired == true)
                  Card(
                      child: Padding(
                          padding: const EdgeInsets.all(16),
                          child: Semantics(
                              liveRegion: true,
                              child: Text(
                                  'Recovery review required. No automatic repair.',
                                  style: TextStyle(
                                      color: theme.colorScheme.error))))),
                const SizedBox(height: 12),
                LayoutBuilder(builder: (context, constraints) {
                  final columns = constraints.maxWidth >= 720
                      ? 3
                      : constraints.maxWidth >= 430
                          ? 2
                          : 1;
                  final width =
                      (constraints.maxWidth - 12 * (columns - 1)) / columns;
                  return Wrap(spacing: 12, runSpacing: 12, children: [
                    for (final item in items)
                      SizedBox(
                          width: width,
                          child: Card(
                              child: Padding(
                                  padding: const EdgeInsets.all(18),
                                  child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.start,
                                      children: [
                                        Icon(item.$3),
                                        const SizedBox(height: 8),
                                        Text(item.$1),
                                        Text(
                                            item.$2?.toString() ??
                                                'Unavailable',
                                            style: item.$2 == null
                                                ? theme.textTheme.titleMedium
                                                : theme
                                                    .textTheme.headlineMedium)
                                      ])))),
                  ]);
                }),
                const SizedBox(height: 18),
                FilledButton.icon(
                    onPressed: busy || monitoring || widget.source == null
                        ? null
                        : refresh,
                    icon: const Icon(Icons.refresh),
                    label: const Text('Refresh locally')),
              ] else ...[
                const Text(
                    'The dashboard calls only the existing privacy-minimized local-query runtime-summary command.'),
                const SizedBox(height: 16),
                for (final text in [
                  'No container identifiers, paths, PIDs or annotations displayed.',
                  'No remote network, analytics or credential discovery.',
                  'Monitoring is explicitly started, bounded and stops on recovery or leaving the overview.',
                  'No deployment, mutation or automatic recovery.',
                  'Same-user Unix access is not GoreeCloud Identity authorization.',
                  'Runtime observations are not an atomic snapshot.',
                  'Stable production and MIPS acceptance are not established.',
                ])
                  Card(
                      child: ListTile(
                          leading: const Icon(Icons.info_outline),
                          title: Text(text))),
              ],
            ])),
      ))),
    );
  }
}
