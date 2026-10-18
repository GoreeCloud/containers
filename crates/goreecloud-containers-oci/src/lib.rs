use std::error::Error;
use std::fmt;
use std::fmt::Write as _;
use std::fs::{self, OpenOptions};
use std::io::{self, Write};
use std::path::{Path, PathBuf};

pub const OCI_RUNTIME_SPEC_VERSION: &str = "1.3.0";
pub const OCI_CONFIG_FILENAME: &str = "config.json";
pub const DEFAULT_ROOTFS_DIRECTORY: &str = "rootfs";
const DEFAULT_CONTAINER_UID: u32 = 65_534;
const DEFAULT_CONTAINER_GID: u32 = 65_534;

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OciUser {
    pub uid: u32,
    pub gid: u32,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OciProcess {
    pub terminal: bool,
    pub user: OciUser,
    pub args: Vec<String>,
    pub env: Vec<String>,
    pub cwd: String,
    pub no_new_privileges: bool,
}

impl OciProcess {
    pub fn minimal(args: Vec<String>) -> Result<Self, OciConfigError> {
        if args.is_empty() {
            return Err(OciConfigError::MissingProcessArgs);
        }
        if args.iter().any(|argument| argument.is_empty()) {
            return Err(OciConfigError::EmptyProcessArg);
        }
        if args.iter().any(|argument| argument.contains('\0')) {
            return Err(OciConfigError::InvalidProcessArg);
        }

        Ok(Self {
            terminal: false,
            user: OciUser {
                uid: DEFAULT_CONTAINER_UID,
                gid: DEFAULT_CONTAINER_GID,
            },
            args,
            env: vec![
                "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin".to_owned(),
            ],
            cwd: "/".to_owned(),
            no_new_privileges: true,
        })
    }

    pub fn with_user(mut self, uid: u32, gid: u32) -> Self {
        self.user = OciUser { uid, gid };
        self
    }

    pub fn with_env(mut self, env: Vec<String>) -> Result<Self, OciConfigError> {
        if env.iter().any(|entry| {
            if entry.contains('\0') {
                return true;
            }
            let Some((name, _)) = entry.split_once('=') else {
                return true;
            };
            name.is_empty()
        }) {
            return Err(OciConfigError::InvalidEnvironmentEntry);
        }
        self.env = env;
        Ok(self)
    }

    pub fn with_cwd(mut self, cwd: impl Into<String>) -> Result<Self, OciConfigError> {
        let cwd = cwd.into();
        if cwd.contains('\0') {
            return Err(OciConfigError::InvalidCwd(cwd));
        }
        if !cwd.starts_with('/') {
            return Err(OciConfigError::CwdNotAbsolute(cwd));
        }
        self.cwd = cwd;
        Ok(self)
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OciRoot {
    pub path: String,
    pub readonly: bool,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum LinuxNamespaceType {
    Pid,
    Network,
    Ipc,
    Uts,
    Mount,
    User,
}

impl LinuxNamespaceType {
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Pid => "pid",
            Self::Network => "network",
            Self::Ipc => "ipc",
            Self::Uts => "uts",
            Self::Mount => "mount",
            Self::User => "user",
        }
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OciIdMapping {
    pub container_id: u32,
    pub host_id: u32,
    pub size: u32,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OciLinux {
    pub namespaces: Vec<LinuxNamespaceType>,
    pub uid_mappings: Vec<OciIdMapping>,
    pub gid_mappings: Vec<OciIdMapping>,
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct OciConfig {
    pub oci_version: String,
    pub process: OciProcess,
    pub root: OciRoot,
    pub hostname: Option<String>,
    pub linux: OciLinux,
}

impl OciConfig {
    pub fn minimal_linux(args: Vec<String>) -> Result<Self, OciConfigError> {
        Ok(Self {
            oci_version: OCI_RUNTIME_SPEC_VERSION.to_owned(),
            process: OciProcess::minimal(args)?,
            root: OciRoot {
                path: DEFAULT_ROOTFS_DIRECTORY.to_owned(),
                readonly: false,
            },
            hostname: Some("goree-container".to_owned()),
            linux: OciLinux {
                namespaces: vec![
                    LinuxNamespaceType::Pid,
                    LinuxNamespaceType::Network,
                    LinuxNamespaceType::Ipc,
                    LinuxNamespaceType::Uts,
                    LinuxNamespaceType::Mount,
                ],
                uid_mappings: Vec::new(),
                gid_mappings: Vec::new(),
            },
        })
    }

    /// Construct a Development-only rootless OCI profile with a single
    /// subordinate mapping of container root to the invoking host user.
    ///
    /// Real runtime lifecycle, filesystem, and isolation acceptance remain
    /// separate requirements; this only builds an OCI configuration.
    pub fn minimal_rootless_linux(
        args: Vec<String>,
        host_uid: u32,
        host_gid: u32,
    ) -> Result<Self, OciConfigError> {
        if host_uid == 0 || host_gid == 0 {
            return Err(OciConfigError::RootlessHostIdMustBeUnprivileged);
        }
        let mut config = Self::minimal_linux(args)?;
        config.process.user = OciUser { uid: 0, gid: 0 };
        config.root.readonly = true;
        config.linux.namespaces.push(LinuxNamespaceType::User);
        config.linux.uid_mappings = vec![OciIdMapping {
            container_id: 0,
            host_id: host_uid,
            size: 1,
        }];
        config.linux.gid_mappings = vec![OciIdMapping {
            container_id: 0,
            host_id: host_gid,
            size: 1,
        }];
        Ok(config)
    }

    #[must_use]
    pub fn to_json_pretty(&self) -> String {
        let mut output = String::new();
        output.push_str("{\n");
        output.push_str("  \"ociVersion\": ");
        push_json_string(&mut output, &self.oci_version);
        output.push_str(",\n  \"process\": {\n");
        output.push_str("    \"terminal\": ");
        output.push_str(if self.process.terminal {
            "true"
        } else {
            "false"
        });
        output.push_str(",\n    \"user\": {\"uid\": ");
        let _ = write!(output, "{}", self.process.user.uid);
        output.push_str(", \"gid\": ");
        let _ = write!(output, "{}", self.process.user.gid);
        output.push_str("},\n    \"args\": ");
        push_json_string_array(&mut output, &self.process.args);
        output.push_str(",\n    \"env\": ");
        push_json_string_array(&mut output, &self.process.env);
        output.push_str(",\n    \"cwd\": ");
        push_json_string(&mut output, &self.process.cwd);
        output.push_str(",\n    \"noNewPrivileges\": ");
        output.push_str(if self.process.no_new_privileges {
            "true"
        } else {
            "false"
        });
        output.push_str("\n  },\n  \"root\": {\n    \"path\": ");
        push_json_string(&mut output, &self.root.path);
        output.push_str(",\n    \"readonly\": ");
        output.push_str(if self.root.readonly { "true" } else { "false" });
        output.push_str("\n  }");

        if let Some(hostname) = &self.hostname {
            output.push_str(",\n  \"hostname\": ");
            push_json_string(&mut output, hostname);
        }

        output.push_str(",\n  \"linux\": {\n    \"namespaces\": [");
        for (index, namespace) in self.linux.namespaces.iter().enumerate() {
            if index != 0 {
                output.push_str(", ");
            }
            output.push_str("{\"type\": ");
            push_json_string(&mut output, namespace.as_str());
            output.push('}');
        }
        output.push(']');
        if !self.linux.uid_mappings.is_empty() {
            output.push_str(",\n    \"uidMappings\": ");
            push_id_mappings(&mut output, &self.linux.uid_mappings);
        }
        if !self.linux.gid_mappings.is_empty() {
            output.push_str(",\n    \"gidMappings\": ");
            push_id_mappings(&mut output, &self.linux.gid_mappings);
        }
        output.push_str("\n  }\n}\n");
        output
    }
}

#[derive(Clone, Debug, Eq, PartialEq)]
pub struct InitializedBundle {
    pub bundle_path: PathBuf,
    pub rootfs_path: PathBuf,
    pub config_path: PathBuf,
}

#[derive(Debug)]
pub enum OciConfigError {
    MissingProcessArgs,
    EmptyProcessArg,
    InvalidProcessArg,
    RootlessHostIdMustBeUnprivileged,
    InvalidEnvironmentEntry,
    InvalidCwd(String),
    CwdNotAbsolute(String),
    UnsafeRootPath(String),
    BundlePathNotAbsolute(PathBuf),
    BundlePathSymlink(PathBuf),
    BundleNotDirectory(PathBuf),
    RootfsPathSymlink(PathBuf),
    RootfsNotDirectory(PathBuf),
    ConfigAlreadyExists(PathBuf),
    Io {
        operation: &'static str,
        path: PathBuf,
        source: io::Error,
    },
}

impl fmt::Display for OciConfigError {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::MissingProcessArgs => {
                formatter.write_str("OCI process arguments must contain a command")
            }
            Self::EmptyProcessArg => {
                formatter.write_str("OCI process arguments must not contain empty values")
            }
            Self::InvalidProcessArg => {
                formatter.write_str("OCI process arguments must not contain NUL bytes")
            }
            Self::RootlessHostIdMustBeUnprivileged => {
                formatter.write_str("rootless host UID and GID must both be nonzero")
            }
            Self::InvalidEnvironmentEntry => {
                formatter.write_str("OCI environment entries must use non-empty NAME=VALUE form")
            }
            Self::InvalidCwd(_) => formatter.write_str("OCI cwd contains invalid NUL"),
            Self::UnsafeRootPath(_) => formatter.write_str("OCI root path is not bundle rootfs"),
            Self::CwdNotAbsolute(cwd) => {
                write!(formatter, "OCI process cwd must be absolute: {cwd}")
            }
            Self::BundlePathNotAbsolute(path) => write!(
                formatter,
                "OCI bundle path must be absolute to avoid working-directory ambiguity: {}",
                path.display()
            ),
            Self::BundlePathSymlink(path) => write!(
                formatter,
                "OCI bundle path must not itself be a symbolic link: {}",
                path.display()
            ),
            Self::BundleNotDirectory(path) => {
                write!(
                    formatter,
                    "OCI bundle path is not a directory: {}",
                    path.display()
                )
            }
            Self::RootfsPathSymlink(path) => write!(
                formatter,
                "OCI root filesystem path must not itself be a symbolic link: {}",
                path.display()
            ),
            Self::RootfsNotDirectory(path) => write!(
                formatter,
                "OCI root filesystem path is not a directory: {}",
                path.display()
            ),
            Self::ConfigAlreadyExists(path) => write!(
                formatter,
                "refusing to overwrite existing OCI configuration: {}",
                path.display()
            ),
            Self::Io {
                operation,
                path,
                source,
            } => write!(
                formatter,
                "failed to {operation} '{}': {source}",
                path.display()
            ),
        }
    }
}

impl Error for OciConfigError {
    fn source(&self) -> Option<&(dyn Error + 'static)> {
        match self {
            Self::Io { source, .. } => Some(source),
            _ => None,
        }
    }
}

pub fn initialize_linux_bundle(
    bundle_path: &Path,
    config: &OciConfig,
) -> Result<InitializedBundle, OciConfigError> {
    if !bundle_path.is_absolute() {
        return Err(OciConfigError::BundlePathNotAbsolute(
            bundle_path.to_path_buf(),
        ));
    }

    if config.root.path != DEFAULT_ROOTFS_DIRECTORY {
        return Err(OciConfigError::UnsafeRootPath(config.root.path.clone()));
    }
    // These fields are public: validate again even when a caller bypasses the
    // checked constructors by changing a configuration after construction.
    OciProcess::minimal(config.process.args.clone())?;
    config.process.clone().with_env(config.process.env.clone())?;
    config.process.clone().with_cwd(config.process.cwd.clone())?;

    let bundle_metadata =
        fs::symlink_metadata(bundle_path).map_err(|source| OciConfigError::Io {
            operation: "inspect OCI bundle",
            path: bundle_path.to_path_buf(),
            source,
        })?;
    if bundle_metadata.file_type().is_symlink() {
        return Err(OciConfigError::BundlePathSymlink(bundle_path.to_path_buf()));
    }
    if !bundle_metadata.is_dir() {
        return Err(OciConfigError::BundleNotDirectory(
            bundle_path.to_path_buf(),
        ));
    }

    let canonical_bundle = fs::canonicalize(bundle_path).map_err(|source| OciConfigError::Io {
        operation: "canonicalize OCI bundle",
        path: bundle_path.to_path_buf(),
        source,
    })?;
    let rootfs_path = canonical_bundle.join(DEFAULT_ROOTFS_DIRECTORY);
    let rootfs_metadata =
        fs::symlink_metadata(&rootfs_path).map_err(|source| OciConfigError::Io {
            operation: "inspect OCI root filesystem",
            path: rootfs_path.clone(),
            source,
        })?;
    if rootfs_metadata.file_type().is_symlink() {
        return Err(OciConfigError::RootfsPathSymlink(rootfs_path));
    }
    if !rootfs_metadata.is_dir() {
        return Err(OciConfigError::RootfsNotDirectory(rootfs_path));
    }

    let config_path = canonical_bundle.join(OCI_CONFIG_FILENAME);
    match fs::symlink_metadata(&config_path) {
        Ok(_) => return Err(OciConfigError::ConfigAlreadyExists(config_path)),
        Err(error) if error.kind() == io::ErrorKind::NotFound => {}
        Err(source) => {
            return Err(OciConfigError::Io {
                operation: "inspect OCI configuration",
                path: config_path,
                source,
            });
        }
    }

    let mut options = OpenOptions::new();
    options.write(true).create_new(true);
    // OCI configurations may contain process environment values. Never expose
    // them through a permissive host umask when creating a new bundle.
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt as _;
        options.mode(0o600);
    }

    let mut file = options.open(&config_path).map_err(|source| {
        if source.kind() == io::ErrorKind::AlreadyExists {
            OciConfigError::ConfigAlreadyExists(config_path.clone())
        } else {
            OciConfigError::Io {
                operation: "create OCI configuration",
                path: config_path.clone(),
                source,
            }
        }
    })?;

    file.write_all(config.to_json_pretty().as_bytes())
        .map_err(|source| OciConfigError::Io {
            operation: "write OCI configuration",
            path: config_path.clone(),
            source,
        })?;
    file.sync_all().map_err(|source| OciConfigError::Io {
        operation: "synchronize OCI configuration",
        path: config_path.clone(),
        source,
    })?;

    Ok(InitializedBundle {
        bundle_path: canonical_bundle,
        rootfs_path,
        config_path,
    })
}

fn push_id_mappings(output: &mut String, mappings: &[OciIdMapping]) {
    output.push('[');
    for (index, mapping) in mappings.iter().enumerate() {
        if index != 0 {
            output.push_str(", ");
        }
        let _ = write!(
            output,
            "{{\"containerID\": {}, \"hostID\": {}, \"size\": {}}}",
            mapping.container_id, mapping.host_id, mapping.size
        );
    }
    output.push(']');
}

fn push_json_string_array(output: &mut String, values: &[String]) {
    output.push('[');
    for (index, value) in values.iter().enumerate() {
        if index != 0 {
            output.push_str(", ");
        }
        push_json_string(output, value);
    }
    output.push(']');
}

fn push_json_string(output: &mut String, value: &str) {
    output.push('"');
    for character in value.chars() {
        match character {
            '"' => output.push_str("\\\""),
            '\\' => output.push_str("\\\\"),
            '\u{0008}' => output.push_str("\\b"),
            '\u{000c}' => output.push_str("\\f"),
            '\n' => output.push_str("\\n"),
            '\r' => output.push_str("\\r"),
            '\t' => output.push_str("\\t"),
            character if character <= '\u{001f}' => {
                let _ = write!(output, "\\u{:04x}", u32::from(character));
            }
            character => output.push(character),
        }
    }
    output.push('"');
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicU64, Ordering};

    static NEXT_TEST_DIRECTORY: AtomicU64 = AtomicU64::new(1);

    fn temporary_directory(test_name: &str) -> PathBuf {
        let sequence = NEXT_TEST_DIRECTORY.fetch_add(1, Ordering::Relaxed);
        std::env::temp_dir().join(format!(
            "goreecloud-containers-oci-{test_name}-{}-{sequence}",
            std::process::id()
        ))
    }

    #[test]
    fn serializes_minimal_linux_config_with_required_foundation_fields() {
        let config = match OciConfig::minimal_linux(vec![
            "/bin/echo".to_owned(),
            "hello \"GoreeCloud\"".to_owned(),
        ]) {
            Ok(config) => config,
            Err(error) => panic!("minimal config should be valid: {error}"),
        };

        let json = config.to_json_pretty();
        assert!(json.contains("\"ociVersion\": \"1.3.0\""));
        assert!(json.contains("\"noNewPrivileges\": true"));
        assert!(json.contains("\"path\": \"rootfs\""));
        assert!(json.contains("hello \\\"GoreeCloud\\\""));
        assert!(json.ends_with("}\n"));
    }

    #[test]
    fn rejects_unsafe_oci_root_without_creating_config() {
        let directory = temporary_directory("unsafe-root");
        fs::create_dir_all(directory.join(DEFAULT_ROOTFS_DIRECTORY))
            .expect("create test rootfs");
        let mut config = OciConfig::minimal_linux(vec!["/bin/true".to_owned()])
            .expect("valid minimal config");
        config.root.path = "../outside".to_owned();
        assert!(matches!(
            initialize_linux_bundle(&directory, &config),
            Err(OciConfigError::UnsafeRootPath(_))
        ));
        assert!(!directory.join(OCI_CONFIG_FILENAME).exists());
        fs::remove_dir_all(directory).expect("cleanup test directory");
    }

    #[test]
    fn rejects_invalid_mutable_process_fields_before_publication() {
        let directory = temporary_directory("invalid-process-fields");
        fs::create_dir_all(directory.join(DEFAULT_ROOTFS_DIRECTORY))
            .expect("create test rootfs");
        let base = OciConfig::minimal_linux(vec!["/bin/true".to_owned()])
            .expect("valid minimal config");

        let mut invalid = base.clone();
        invalid.process.args = vec!["/bin/true\0extra".to_owned()];
        assert!(matches!(
            initialize_linux_bundle(&directory, &invalid),
            Err(OciConfigError::InvalidProcessArg)
        ));

        invalid = base.clone();
        invalid.process.env = vec!["TEST=good\0bad".to_owned()];
        assert!(matches!(
            initialize_linux_bundle(&directory, &invalid),
            Err(OciConfigError::InvalidEnvironmentEntry)
        ));

        invalid = base;
        invalid.process.cwd = "/work\0bad".to_owned();
        assert!(matches!(
            initialize_linux_bundle(&directory, &invalid),
            Err(OciConfigError::InvalidCwd(_))
        ));

        assert!(!directory.join(OCI_CONFIG_FILENAME).exists());
        fs::remove_dir_all(directory).expect("cleanup test directory");
    }

    #[test]
    fn rootless_profile_maps_container_root_to_nonroot_host_identity() {
        let config = OciConfig::minimal_rootless_linux(vec!["/bin/true".to_owned()], 1001, 1002)
            .expect("non-root host identities should be accepted");

        assert_eq!(config.process.user, OciUser { uid: 0, gid: 0 });
        assert!(config.process.no_new_privileges);
        assert!(config.root.readonly);
        assert!(config.linux.namespaces.contains(&LinuxNamespaceType::User));
        assert_eq!(
            config.linux.uid_mappings,
            vec![OciIdMapping {
                container_id: 0,
                host_id: 1001,
                size: 1
            }]
        );
        assert_eq!(
            config.linux.gid_mappings,
            vec![OciIdMapping {
                container_id: 0,
                host_id: 1002,
                size: 1
            }]
        );
        let json = config.to_json_pretty();
        assert!(json.contains("\"type\": \"user\""));
        assert!(
            json.contains("\"uidMappings\": [{\"containerID\": 0, \"hostID\": 1001, \"size\": 1}]")
        );
        assert!(
            json.contains("\"gidMappings\": [{\"containerID\": 0, \"hostID\": 1002, \"size\": 1}]")
        );
    }

    #[test]
    fn rootless_profile_rejects_host_root_identities() {
        let args = vec!["/bin/true".to_owned()];
        for (uid, gid) in [(0, 1000), (1000, 0), (0, 0)] {
            assert!(matches!(
                OciConfig::minimal_rootless_linux(args.clone(), uid, gid),
                Err(OciConfigError::RootlessHostIdMustBeUnprivileged)
            ));
        }

        let ordinary = OciConfig::minimal_linux(args).expect("ordinary config should work");
        assert!(ordinary.linux.uid_mappings.is_empty());
        assert!(ordinary.linux.gid_mappings.is_empty());
        assert!(!ordinary.to_json_pretty().contains("\"uidMappings\""));
        assert!(!ordinary.to_json_pretty().contains("\"gidMappings\""));
    }

    #[test]
    fn rejects_missing_process_command() {
        assert!(matches!(
            OciConfig::minimal_linux(Vec::new()),
            Err(OciConfigError::MissingProcessArgs)
        ));
    }

    #[cfg(unix)]
    #[test]
    fn newly_created_bundle_config_is_owner_only() {
        use std::os::unix::fs::PermissionsExt as _;

        let directory = temporary_directory("private-config");
        let rootfs = directory.join(DEFAULT_ROOTFS_DIRECTORY);
        fs::create_dir_all(&rootfs).expect("rootfs fixture should exist");

        let config = OciConfig::minimal_linux(vec!["/bin/true".to_owned()])
            .expect("minimal OCI config should be valid");
        let initialized =
            initialize_linux_bundle(&directory, &config).expect("OCI bundle should initialize");

        let permissions = fs::metadata(&initialized.config_path)
            .expect("config metadata should be readable")
            .permissions()
            .mode()
            & 0o777;
        assert_eq!(permissions, 0o600);

        fs::remove_dir_all(&directory).expect("test directory should be removable");
    }

    #[test]
    fn initializes_bundle_without_overwriting_existing_config() {
        let directory = temporary_directory("init");
        let rootfs = directory.join(DEFAULT_ROOTFS_DIRECTORY);
        assert!(fs::create_dir_all(&rootfs).is_ok());

        let config = match OciConfig::minimal_linux(vec!["/bin/true".to_owned()]) {
            Ok(config) => config,
            Err(error) => panic!("test config should be valid: {error}"),
        };

        let first = initialize_linux_bundle(&directory, &config);
        assert!(first.is_ok());
        assert!(directory.join(OCI_CONFIG_FILENAME).is_file());

        let second = initialize_linux_bundle(&directory, &config);
        assert!(matches!(
            second,
            Err(OciConfigError::ConfigAlreadyExists(_))
        ));

        assert!(fs::remove_dir_all(directory).is_ok());
    }
}
