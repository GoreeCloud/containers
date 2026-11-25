/* First-party isolated acceptance fixture. No external data or network use. */
#define _GNU_SOURCE
#include <errno.h>
#include <fcntl.h>
#include <linux/capability.h>
#include <signal.h>
#include <sys/prctl.h>
#include <sys/socket.h>
#include <sys/syscall.h>
#include <time.h>
#include <unistd.h>

static volatile sig_atomic_t stop;
static void on_term(int signal_number) { (void)signal_number; stop = 1; }

int main(void) {
    if (getpid() != 1 || getuid() != 0 || getgid() != 0) return 11;
    if (prctl(PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0) != 1) return 12;
    if (prctl(PR_GET_SECCOMP, 0, 0, 0, 0) != 2) return 13;
    struct __user_cap_header_struct h = { _LINUX_CAPABILITY_VERSION_3, 0 };
    struct __user_cap_data_struct caps[2] = {0};
    if (syscall(SYS_capget, &h, caps) != 0) return 14;
    for (int i = 0; i < 2; i++)
        if (caps[i].effective || caps[i].permitted || caps[i].inheritable) return 15;
    errno = 0;
    int fd = open("/readonly-probe", O_CREAT | O_EXCL | O_WRONLY, 0600);
    if (fd >= 0) { close(fd); return 16; }
    if (errno != EROFS) return 17;
    errno = 0;
    fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd >= 0) { close(fd); return 18; }
    if (errno != EPERM) return 19;
    struct sigaction action = {0};
    action.sa_handler = on_term;
    if (sigaction(SIGTERM, &action, 0) != 0) return 20;
    /* Remain alive only after the checks; self-terminate if the caller dies. */
    for (int i = 0; i < 300 && !stop; i++) {
        struct timespec delay = {0, 100000000L};
        nanosleep(&delay, 0);
    }
    return stop ? 0 : 21;
}
