# Stop completion evidence

The journaled SIGTERM operation waits up to three seconds (or the caller deadline, if earlier) for a trusted runtime state reporting stopped with PID zero. An unavailable, unexpected or still-running state preserves recovery evidence. No automatic SIGKILL, forced delete or marker clearing is performed. Delayed-stop, unexpected-state, unavailable-state and timeout regressions passed, as did the real isolated snapshot lifecycle with six namespaces and cgroup readback. Workloads with longer shutdown deadlines need an explicit future supervision policy.

The common journal commit guard now also checks Linux inode change time and exact mode, rejecting an in-place rewrite even with the original modification time restored. The regression preserves the pending marker; this is not atomic compare-and-unlink or malicious same-UID protection.
