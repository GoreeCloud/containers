package bundle

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

const safeSpec = `{
 "ociVersion":"1.2.0",
 "process":{"args":["/bin/true"],"cwd":"/","user":{"uid":0,"gid":0},
 "capabilities":{"bounding":[],"effective":[],"inheritable":[],"permitted":[],"ambient":[]},
 "noNewPrivileges":true,"terminal":false},
 "root":{"path":"rootfs","readonly":true},
 "linux":{"namespaces":[{"type":"user"},{"type":"pid"},{"type":"mount"},{"type":"network"},{"type":"ipc"},{"type":"uts"}],
 "uidMappings":[{"containerID":0,"hostID":1000,"size":1}],
 "gidMappings":[{"containerID":0,"hostID":1000,"size":1}],
 "seccomp":{"defaultAction":"SCMP_ACT_ERRNO"}}
}`

func setup(t *testing.T, content string) string {
    t.Helper()
    dir := t.TempDir()
    if err := os.Mkdir(filepath.Join(dir,"rootfs"),0700); err != nil { t.Fatal(err) }
    if err := os.WriteFile(filepath.Join(dir,"config.json"),[]byte(content),0600); err != nil { t.Fatal(err) }
    return dir
}
func TestValidRootlessBundle(t *testing.T) {
    if err := Validate(setup(t,safeSpec)); err != nil { t.Fatal(err) }
}
func TestFailClosed(t *testing.T) {
    cases := map[string]string{
       "unknown field": strings.Replace(safeSpec,`"ociVersion"`,`"surprise":true,"ociVersion"`,1),
       "duplicate": strings.Replace(safeSpec,`"ociVersion"`,`"ociVersion":"1.0.0","ociVersion"`,1),
       "missing namespace": strings.Replace(safeSpec,`,{"type":"user"}`,`},`,1),
       "host root mapping": strings.Replace(safeSpec,`"hostID":1000`,`"hostID":0`,1),
       "no seccomp": strings.Replace(safeSpec,`"defaultAction":"SCMP_ACT_ERRNO"`,`"defaultAction":"SCMP_ACT_ALLOW"`,1),
       "writable root": strings.Replace(safeSpec,`"readonly":true`,`"readonly":false`,1),
       "capabilities": strings.Replace(safeSpec,`"bounding":[]`,`"bounding":["CAP_SYS_ADMIN"]`,1),
       "privilege": strings.Replace(safeSpec,`"noNewPrivileges":true`,`"noNewPrivileges":false`,1),
       "external root": strings.Replace(safeSpec,`"path":"rootfs"`,`"path":"../rootfs"`,1),
       "hook": strings.Replace(safeSpec,`"root":`,`"hooks":{"prestart":[]},"root":`,1),
       "bind mount": strings.Replace(safeSpec,`"linux":`,`"mounts":[{"destination":"/tmp","type":"bind","source":"/"}],"linux":`,1),
    }
    for name, raw := range cases {
        t.Run(name,func(t *testing.T){
            if name=="missing namespace" {
                raw = strings.Replace(safeSpec,`{"type":"user"},`,"",1)
            }
            if err := Validate(setup(t,raw)); err == nil { t.Fatal("insecure bundle accepted") }
        })
    }
}
func TestTooLargeAndSymlink(t *testing.T) {
    dir := setup(t,safeSpec+strings.Repeat(" ",MaxConfigBytes))
    if err := Validate(dir); err == nil { t.Fatal("oversize accepted") }
    target := setup(t,safeSpec)
    link := filepath.Join(t.TempDir(),"config.json")
    if err := os.Symlink(filepath.Join(target,"config.json"),link); err != nil { t.Fatal(err) }
    if err := os.Remove(filepath.Join(dir,"config.json")); err != nil { t.Fatal(err) }
    if err := os.Rename(link,filepath.Join(dir,"config.json")); err != nil { t.Fatal(err) }
    if err := Validate(dir); err == nil { t.Fatal("symlink accepted") }
}
