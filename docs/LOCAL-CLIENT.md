# Local management client preview

`gcontainers local-query ABS_SOCKET health|journal` makes a bounded GET request to a private same-UID Unix socket. Socket ownership, mode, path identity and connected SO_PEERCRED are checked. The client refuses symlinks, arbitrary endpoints, redirects, oversized or invalid JSON, and other HTTP statuses. Journal conflict (409) reports recovery evidence and exits unsuccessfully. No TCP, credential integration or mutations. Same UID is not a verified application identity. Real socket tests passed.
