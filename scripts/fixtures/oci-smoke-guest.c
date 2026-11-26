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
#include <sys/vfs.h>
#ifdef GC_LOCAL_IO_PROBE
#include <netinet/in.h>
#include <string.h>
#include <ctype.h>
#endif
#ifndef GC_GUEST_ID
#define GC_GUEST_ID 0
#endif

#ifdef GC_LOCAL_IO_PROBE
/* Only connect to a socket created in this same isolated namespace. */
static int loopback_only(void) {
    char buf[4096];
    int fd = open("/proc/net/dev", O_RDONLY);
    if (fd < 0) return 0;
    ssize_t n = read(fd, buf, sizeof(buf)-1);
    close(fd);
    if (n <= 0 || n == (ssize_t)(sizeof(buf)-1)) return 0;
    buf[n] = 0;
    int count = 0;
    char *save = NULL;
    for (char *line = strtok_r(buf, "\n", &save); line; line = strtok_r(NULL, "\n", &save)) {
        char *colon = strchr(line, ':');
        if (!colon) continue;
        *colon = 0;
        while (isspace((unsigned char)*line)) line++;
        if (strcmp(line, "lo") != 0) return 0;
        count++;
    }
    return count == 1;
}
static int local_io(void) {
    if (!loopback_only()) return 31;
    int server = socket(AF_INET, SOCK_STREAM | SOCK_CLOEXEC, 0);
    if (server < 0) return 32;
    struct sockaddr_in addr = {.sin_family=AF_INET,.sin_port=0,.sin_addr={.s_addr=htonl(INADDR_LOOPBACK)}};
    if (bind(server,(struct sockaddr*)&addr,sizeof(addr)) || listen(server,1)) return 33;
    socklen_t len=sizeof(addr);
    if (getsockname(server,(struct sockaddr*)&addr,&len) || len!=sizeof(addr) || addr.sin_addr.s_addr!=htonl(INADDR_LOOPBACK) || addr.sin_port==0) return 34;
    int client=socket(AF_INET,SOCK_STREAM|SOCK_CLOEXEC,0);
    if(client<0 || connect(client,(struct sockaddr*)&addr,sizeof(addr))) return 35;
    int peer=accept4(server,NULL,NULL,SOCK_CLOEXEC);
    if(peer<0) return 36;
    const char message[]="goreecloud-local-fixture";
    char received[sizeof(message)];
    if(write(client,message,sizeof(message))!=(ssize_t)sizeof(message))return 37;
    size_t total=0;
    while(total<sizeof(received)){
        ssize_t got=read(peer,received+total,sizeof(received)-total);
        if(got<=0)return 38;
        total+=(size_t)got;
    }
    if(memcmp(message,received,sizeof(message)))return 39;
    close(peer);close(client);close(server);
    int scratch=open("/tmp/gc-local-io-fixture",O_CREAT|O_EXCL|O_RDWR|O_CLOEXEC,0600);
    if(scratch<0)return 40;
    if(write(scratch,message,sizeof(message))!=(ssize_t)sizeof(message)||lseek(scratch,0,SEEK_SET)!=0)return 41;
    if(read(scratch,received,sizeof(received))!=(ssize_t)sizeof(received)||memcmp(message,received,sizeof(message)))return 42;
    close(scratch);
    if(unlink("/tmp/gc-local-io-fixture"))return 43;
    return 0;
}
#endif

static volatile sig_atomic_t stop;
static void on_term(int signal_number) { (void)signal_number; stop = 1; }

int main(void) {
    if (getpid() != 1 || getuid() != GC_GUEST_ID || getgid() != GC_GUEST_ID || geteuid() != GC_GUEST_ID || getegid() != GC_GUEST_ID) return 11;
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
    if (errno != EROFS && (GC_GUEST_ID == 0 || errno != EACCES)) return 17;
    fd = open("/", O_RDONLY | O_DIRECTORY);
    if (fd < 0) return 22;
    struct statfs root;
    if (fstatfs(fd, &root) != 0 || !(root.f_flags & 1)) { close(fd); return 23; }
    close(fd);
    errno = 0;
#ifdef GC_LOCAL_IO_PROBE
    int local_result=local_io();
    if(local_result)return local_result;
#else
    fd = socket(AF_INET, SOCK_STREAM, 0);
    if (fd >= 0) { close(fd); return 18; }
    if (errno != EPERM) return 19;
#endif
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
