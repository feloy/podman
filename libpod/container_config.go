package libpod

import (
	"net"
	"time"

	"github.com/containers/common/libnetwork/types"
	"github.com/containers/common/pkg/secrets"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/podman/v4/pkg/namespaces"
	"github.com/containers/podman/v4/pkg/specgen"
	"github.com/containers/storage"
	spec "github.com/opencontainers/runtime-spec/specs-go"
)

// ContainerConfig contains all information that was used to create the
// container. It may not be changed once created.
// It is stored, read-only, on disk in Libpod's State.
// Any changes will not be written back to the database, and will cause
// inconsistencies with other Libpod instances.
type ContainerConfig struct {
	// Spec is OCI runtime spec used to create the container. This is passed
	// in when the container is created, but it is not the final spec used
	// to run the container - it will be modified by Libpod to add things we
	// manage (e.g. bind mounts for /etc/resolv.conf, named volumes, a
	// network namespace prepared by CNI or slirp4netns) in the
	// generateSpec() function.
	Spec *spec.Spec `json:"spec"`

	// ID is a hex-encoded 256-bit pseudorandom integer used as a unique
	// identifier for the container. IDs are globally unique in Libpod -
	// once an ID is in use, no other container or pod will be created with
	// the same one until the holder of the ID has been removed.
	// ID is generated by Libpod, and cannot be chosen or influenced by the
	// user (except when restoring a checkpointed container).
	// ID is guaranteed to be 64 characters long.
	ID string `json:"id"`

	// Name is a human-readable name for the container. All containers must
	// have a non-empty name. Name may be provided when the container is
	// created; if no name is chosen, a name will be auto-generated.
	Name string `json:"name"`

	// Pod is the full ID of the pod the container belongs to. If the
	// container does not belong to a pod, this will be empty.
	// If this is not empty, a pod with this ID is guaranteed to exist in
	// the state for the duration of this container's existence.
	Pod string `json:"pod,omitempty"`

	// Namespace is the libpod Namespace the container is in.
	// Namespaces are used to divide containers in the state.
	Namespace string `json:"namespace,omitempty"`

	// LockID is the ID of this container's lock. Each container, pod, and
	// volume is assigned a unique Lock (from one of several backends) by
	// the libpod Runtime. This lock will belong only to this container for
	// the duration of the container's lifetime.
	LockID uint32 `json:"lockID"`

	// CreateCommand is the full command plus arguments that were used to
	// create the container. It is shown in the output of Inspect, and may
	// be used to recreate an identical container for automatic updates or
	// portable systemd unit files.
	CreateCommand []string `json:"CreateCommand,omitempty"`

	// RawImageName is the raw and unprocessed name of the image when creating
	// the container (as specified by the user).  May or may not be set.  One
	// use case to store this data are auto-updates where we need the _exact_
	// name and not some normalized instance of it.
	RawImageName string `json:"RawImageName,omitempty"`

	// IDMappings are UID/GID mappings used by the container's user
	// namespace. They are used by the OCI runtime when creating the
	// container, and by c/storage to ensure that the container's files have
	// the appropriate owner.
	IDMappings storage.IDMappingOptions `json:"idMappingsOptions,omitempty"`

	// Dependencies are the IDs of dependency containers.
	// These containers must be started before this container is started.
	Dependencies []string

	// rewrite is an internal bool to indicate that the config was modified after
	// a read from the db, e.g. to migrate config fields after an upgrade.
	// This field should never be written to the db, the json tag ensures this.
	rewrite bool `json:"-"`

	// embedded sub-configs
	ContainerRootFSConfig
	ContainerSecurityConfig
	ContainerNameSpaceConfig
	ContainerNetworkConfig
	ContainerImageConfig
	ContainerMiscConfig
}

// ContainerRootFSConfig is an embedded sub-config providing config info
// about the container's root fs.
type ContainerRootFSConfig struct {
	// RootfsImageID is the ID of the image used to create the container.
	// If the container was created from a Rootfs, this will be empty.
	// If non-empty, Podman will create a root filesystem for the container
	// based on an image with this ID.
	// This conflicts with Rootfs.
	RootfsImageID string `json:"rootfsImageID,omitempty"`
	// RootfsImageName is the (normalized) name of the image used to create
	// the container. If the container was created from a Rootfs, this will
	// be empty.
	RootfsImageName string `json:"rootfsImageName,omitempty"`
	// Rootfs is a directory to use as the container's root filesystem.
	// If RootfsImageID is set, this will be empty.
	// If this is set, Podman will not create a root filesystem for the
	// container based on an image, and will instead use the given directory
	// as the container's root.
	// Conflicts with RootfsImageID.
	Rootfs string `json:"rootfs,omitempty"`
	// RootfsOverlay tells if rootfs has to be mounted as an overlay
	RootfsOverlay bool `json:"rootfs_overlay,omitempty"`
	// ShmDir is the path to be mounted on /dev/shm in container.
	// If not set manually at creation time, Libpod will create a tmpfs
	// with the size specified in ShmSize and populate this with the path of
	// said tmpfs.
	ShmDir string `json:"ShmDir,omitempty"`
	// NoShmShare indicates whether /dev/shm can be shared with other containers
	NoShmShare bool `json:"NOShmShare,omitempty"`
	// NoShm indicates whether a tmpfs should be created and mounted on  /dev/shm
	NoShm bool `json:"NoShm,omitempty"`
	// ShmSize is the size of the container's SHM. Only used if ShmDir was
	// not set manually at time of creation.
	ShmSize int64 `json:"shmSize"`
	// Static directory for container content that will persist across
	// reboot.
	// StaticDir is a persistent directory for Libpod files that will
	// survive system reboot. It is not part of the container's rootfs and
	// is not mounted into the container. It will be removed when the
	// container is removed.
	// Usually used to store container log files, files that will be bind
	// mounted into the container (e.g. the resolv.conf we made for the
	// container), and other per-container content.
	StaticDir string `json:"staticDir"`
	// Mounts contains all additional mounts into the container rootfs.
	// It is presently only used for the container's SHM directory.
	// These must be unmounted before the container's rootfs is unmounted.
	Mounts []string `json:"mounts,omitempty"`
	// NamedVolumes lists the Libpod named volumes to mount into the
	// container. Each named volume is guaranteed to exist so long as this
	// container exists.
	NamedVolumes []*ContainerNamedVolume `json:"namedVolumes,omitempty"`
	// OverlayVolumes lists the overlay volumes to mount into the container.
	OverlayVolumes []*ContainerOverlayVolume `json:"overlayVolumes,omitempty"`
	// ImageVolumes lists the image volumes to mount into the container.
	// Please note that this is named ctrImageVolumes in JSON to
	// distinguish between these and the old `imageVolumes` field in Podman
	// pre-1.8, which was used in very old Podman versions to determine how
	// image volumes were handled in Libpod (support for these eventually
	// moved out of Libpod into pkg/specgen).
	// Please DO NOT re-use the `imageVolumes` name in container JSON again.
	ImageVolumes []*ContainerImageVolume `json:"ctrImageVolumes,omitempty"`
	// CreateWorkingDir indicates that Libpod should create the container's
	// working directory if it does not exist. Some OCI runtimes do this by
	// default, but others do not.
	CreateWorkingDir bool `json:"createWorkingDir,omitempty"`
	// Secrets lists secrets to mount into the container
	Secrets []*ContainerSecret `json:"secrets,omitempty"`
	// SecretPath is the secrets location in storage
	SecretsPath string `json:"secretsPath"`
	// StorageOpts to be used when creating rootfs
	StorageOpts map[string]string `json:"storageOpts"`
	// Volatile specifies whether the container storage can be optimized
	// at the cost of not syncing all the dirty files in memory.
	Volatile bool `json:"volatile,omitempty"`
	// Passwd allows to user to override podman's passwd/group file setup
	Passwd *bool `json:"passwd,omitempty"`
	// ChrootDirs is an additional set of directories that need to be
	// treated as root directories. Standard bind mounts will be mounted
	// into paths relative to these directories.
	ChrootDirs []string `json:"chroot_directories,omitempty"`
}

// ContainerSecurityConfig is an embedded sub-config providing security configuration
// to the container.
type ContainerSecurityConfig struct {
	// Privileged is whether the container is privileged. Privileged
	// containers have lessened security and increased access to the system.
	// Note that this does NOT directly correspond to Podman's --privileged
	// flag - most of the work of that flag is done in creating the OCI spec
	// given to Libpod. This only enables a small subset of the overall
	// operation, mostly around mounting the container image with reduced
	// security.
	Privileged bool `json:"privileged"`
	// ProcessLabel is the SELinux process label for the container.
	ProcessLabel string `json:"ProcessLabel,omitempty"`
	// MountLabel is the SELinux mount label for the container's root
	// filesystem. Only used if the container was created from an image.
	// If not explicitly set, an unused random MLS label will be assigned by
	// containers/storage (but only if SELinux is enabled).
	MountLabel string `json:"MountLabel,omitempty"`
	// LabelOpts are options passed in by the user to setup SELinux labels.
	// These are used by the containers/storage library.
	LabelOpts []string `json:"labelopts,omitempty"`
	// User and group to use in the container. Can be specified as only user
	// (in which case we will attempt to look up the user in the container
	// to determine the appropriate group) or user and group separated by a
	// colon.
	// Can be specified by name or UID/GID.
	// If unset, this will default to UID and GID 0 (root).
	User string `json:"user,omitempty"`
	// Groups are additional groups to add the container's user to. These
	// are resolved within the container using the container's /etc/passwd.
	Groups []string `json:"groups,omitempty"`
	// HostUsers are a list of host user accounts to add to /etc/passwd
	HostUsers []string `json:"HostUsers,omitempty"`
	// AddCurrentUserPasswdEntry indicates that Libpod should ensure that
	// the container's /etc/passwd contains an entry for the user running
	// Libpod - mostly used in rootless containers where the user running
	// Libpod wants to retain their UID inside the container.
	AddCurrentUserPasswdEntry bool `json:"addCurrentUserPasswdEntry,omitempty"`
}

// ContainerNameSpaceConfig is an embedded sub-config providing
// namespace configuration to the container.
type ContainerNameSpaceConfig struct {
	// IDs of container to share namespaces with
	// NetNsCtr conflicts with the CreateNetNS bool
	// These containers are considered dependencies of the given container
	// They must be started before the given container is started
	IPCNsCtr    string `json:"ipcNsCtr,omitempty"`
	MountNsCtr  string `json:"mountNsCtr,omitempty"`
	NetNsCtr    string `json:"netNsCtr,omitempty"`
	PIDNsCtr    string `json:"pidNsCtr,omitempty"`
	UserNsCtr   string `json:"userNsCtr,omitempty"`
	UTSNsCtr    string `json:"utsNsCtr,omitempty"`
	CgroupNsCtr string `json:"cgroupNsCtr,omitempty"`
}

// ContainerNetworkConfig is an embedded sub-config providing network configuration
// to the container.
type ContainerNetworkConfig struct {
	// CreateNetNS indicates that libpod should create and configure a new
	// network namespace for the container.
	// This cannot be set if NetNsCtr is also set.
	CreateNetNS bool `json:"createNetNS"`
	// StaticIP is a static IP to request for the container.
	// This cannot be set unless CreateNetNS is set.
	// If not set, the container will be dynamically assigned an IP by CNI.
	// Deprecated: Do no use this anymore, this is only for DB backwards compat.
	StaticIP net.IP `json:"staticIP,omitempty"`
	// StaticMAC is a static MAC to request for the container.
	// This cannot be set unless CreateNetNS is set.
	// If not set, the container will be dynamically assigned a MAC by CNI.
	// Deprecated: Do no use this anymore, this is only for DB backwards compat.
	StaticMAC types.HardwareAddr `json:"staticMAC,omitempty"`
	// PortMappings are the ports forwarded to the container's network
	// namespace
	// These are not used unless CreateNetNS is true
	PortMappings []types.PortMapping `json:"newPortMappings,omitempty"`
	// OldPortMappings are the ports forwarded to the container's network
	// namespace. As of podman 4.0 this field is deprecated, use PortMappings
	// instead. The db will convert the old ports to the new structure for you.
	// These are not used unless CreateNetNS is true
	OldPortMappings []types.OCICNIPortMapping `json:"portMappings,omitempty"`
	// ExposedPorts are the ports which are exposed but not forwarded
	// into the container.
	// The map key is the port and the string slice contains the protocols,
	// e.g. tcp and udp
	// These are only set when exposed ports are given but not published.
	ExposedPorts map[uint16][]string `json:"exposedPorts,omitempty"`
	// UseImageResolvConf indicates that resolv.conf should not be
	// bind-mounted inside the container.
	// Conflicts with DNSServer, DNSSearch, DNSOption.
	UseImageResolvConf bool
	// DNS servers to use in container resolv.conf
	// Will override servers in host resolv if set
	DNSServer []net.IP `json:"dnsServer,omitempty"`
	// DNS Search domains to use in container resolv.conf
	// Will override search domains in host resolv if set
	DNSSearch []string `json:"dnsSearch,omitempty"`
	// DNS options to be set in container resolv.conf
	// With override options in host resolv if set
	DNSOption []string `json:"dnsOption,omitempty"`
	// UseImageHosts indicates that /etc/hosts should not be
	// bind-mounted inside the container.
	// Conflicts with HostAdd.
	UseImageHosts bool
	// Hosts to add in container
	// Will be appended to host's host file
	HostAdd []string `json:"hostsAdd,omitempty"`
	// Network names with the network specific options.
	// Please note that these can be altered at runtime. The actual list is
	// stored in the DB and should be retrieved from there via c.networks()
	// this value is only used for container create.
	// Added in podman 4.0, previously NetworksDeprecated was used. Make
	// sure to not change the json tags.
	Networks map[string]types.PerNetworkOptions `json:"newNetworks,omitempty"`
	// Network names to add container to. Empty to use default network.
	// Please note that these can be altered at runtime. The actual list is
	// stored in the DB and should be retrieved from there; this is only the
	// set of networks the container was *created* with.
	// Deprecated: Do no use this anymore, this is only for DB backwards compat.
	// Also note that we need to keep the old json tag to decode from DB correctly
	NetworksDeprecated []string `json:"networks,omitempty"`
	// Network mode specified for the default network.
	NetMode namespaces.NetworkMode `json:"networkMode,omitempty"`
	// NetworkOptions are additional options for each network
	NetworkOptions map[string][]string `json:"network_options,omitempty"`
}

// ContainerImageConfig is an embedded sub-config providing image configuration
// to the container.
type ContainerImageConfig struct {
	// UserVolumes contains user-added volume mounts in the container.
	// These will not be added to the container's spec, as it is assumed
	// they are already present in the spec given to Libpod. Instead, it is
	// used when committing containers to generate the VOLUMES field of the
	// image that is created, and for triggering some OCI hooks which do not
	// fire unless user-added volume mounts are present.
	UserVolumes []string `json:"userVolumes,omitempty"`
	// Entrypoint is the container's entrypoint.
	// It is not used in spec generation, but will be used when the
	// container is committed to populate the entrypoint of the new image.
	Entrypoint []string `json:"entrypoint,omitempty"`
	// Command is the container's command.
	// It is not used in spec generation, but will be used when the
	// container is committed to populate the command of the new image.
	Command []string `json:"command,omitempty"`
}

// ContainerMiscConfig is an embedded sub-config providing misc configuration
// to the container.
type ContainerMiscConfig struct {
	// Whether to keep container STDIN open
	Stdin bool `json:"stdin,omitempty"`
	// Labels is a set of key-value pairs providing additional information
	// about a container
	Labels map[string]string `json:"labels,omitempty"`
	// StopSignal is the signal that will be used to stop the container
	StopSignal uint `json:"stopSignal,omitempty"`
	// StopTimeout is the signal that will be used to stop the container
	StopTimeout uint `json:"stopTimeout,omitempty"`
	// Timeout is maximum time a container will run before getting the kill signal
	Timeout uint `json:"timeout,omitempty"`
	// Time container was created
	CreatedTime time.Time `json:"createdTime"`
	// CgroupManager is the cgroup manager used to create this container.
	// If empty, the runtime default will be used.
	CgroupManager string `json:"cgroupManager,omitempty"`
	// NoCgroups indicates that the container will not create Cgroups. It is
	// incompatible with CgroupParent.  Deprecated in favor of CgroupsMode.
	NoCgroups bool `json:"noCgroups,omitempty"`
	// CgroupsMode indicates how the container will create cgroups
	// (disabled, no-conmon, enabled).  It supersedes NoCgroups.
	CgroupsMode string `json:"cgroupsMode,omitempty"`
	// Cgroup parent of the container.
	CgroupParent string `json:"cgroupParent"`
	// LogPath log location
	LogPath string `json:"logPath"`
	// LogTag is the tag used for logging
	LogTag string `json:"logTag"`
	// LogSize is the tag used for logging
	LogSize int64 `json:"logSize"`
	// LogDriver driver for logs
	LogDriver string `json:"logDriver"`
	// File containing the conmon PID
	ConmonPidFile string `json:"conmonPidFile,omitempty"`
	// RestartPolicy indicates what action the container will take upon
	// exiting naturally.
	// Allowed options are "no" (take no action), "on-failure" (restart on
	// non-zero exit code, up an a maximum of RestartRetries times),
	// and "always" (always restart the container on any exit code).
	// The empty string is treated as the default ("no")
	RestartPolicy string `json:"restart_policy,omitempty"`
	// RestartRetries indicates the number of attempts that will be made to
	// restart the container. Used only if RestartPolicy is set to
	// "on-failure".
	RestartRetries uint `json:"restart_retries,omitempty"`
	// PostConfigureNetNS needed when a user namespace is created by an OCI runtime
	// if the network namespace is created before the user namespace it will be
	// owned by the wrong user namespace.
	PostConfigureNetNS bool `json:"postConfigureNetNS"`
	// OCIRuntime used to create the container
	OCIRuntime string `json:"runtime,omitempty"`
	// IsInfra is a bool indicating whether this container is an infra container used for
	// sharing kernel namespaces in a pod
	IsInfra bool `json:"pause"`
	// IsService is a bool indicating whether this container is a service container used for
	// tracking the life cycle of K8s service.
	IsService bool `json:"isService"`
	// SdNotifyMode tells libpod what to do with a NOTIFY_SOCKET if passed
	SdNotifyMode string `json:"sdnotifyMode,omitempty"`
	// Systemd tells libpod to setup the container in systemd mode, a value of nil denotes false
	Systemd *bool `json:"systemd,omitempty"`
	// HealthCheckConfig has the health check command and related timings
	HealthCheckConfig *manifest.Schema2HealthConfig `json:"healthcheck"`
	// PreserveFDs is a number of additional file descriptors (in addition
	// to 0, 1, 2) that will be passed to the executed process. The total FDs
	// passed will be 3 + PreserveFDs.
	PreserveFDs uint `json:"preserveFds,omitempty"`
	// Timezone is the timezone inside the container.
	// Local means it has the same timezone as the host machine
	Timezone string `json:"timezone,omitempty"`
	// Umask is the umask inside the container.
	Umask string `json:"umask,omitempty"`
	// PidFile is the file that saves the pid of the container process
	PidFile string `json:"pid_file,omitempty"`
	// CDIDevices contains devices that use the CDI
	CDIDevices []string `json:"cdiDevices,omitempty"`
	// DeviceHostSrc contains the original source on the host
	DeviceHostSrc []spec.LinuxDevice `json:"device_host_src,omitempty"`
	// EnvSecrets are secrets that are set as environment variables
	EnvSecrets map[string]*secrets.Secret `json:"secret_env,omitempty"`
	// InitContainerType specifies if the container is an initcontainer
	// and if so, what type: always or once are possible non-nil entries
	InitContainerType string `json:"init_container_type,omitempty"`
	// PasswdEntry specifies arbitrary data to append to a file.
	PasswdEntry string `json:"passwd_entry,omitempty"`
	// MountAllDevices is an option to indicate whether a privileged container
	// will mount all the host's devices
	MountAllDevices bool `json:"mountAllDevices"`
}

// InfraInherit contains the compatible options inheritable from the infra container
type InfraInherit struct {
	ApparmorProfile    string                   `json:"apparmor_profile,omitempty"`
	CapAdd             []string                 `json:"cap_add,omitempty"`
	CapDrop            []string                 `json:"cap_drop,omitempty"`
	HostDeviceList     []spec.LinuxDevice       `json:"host_device_list,omitempty"`
	ImageVolumes       []*specgen.ImageVolume   `json:"image_volumes,omitempty"`
	InfraResources     *spec.LinuxResources     `json:"resource_limits,omitempty"`
	Mounts             []spec.Mount             `json:"mounts,omitempty"`
	NoNewPrivileges    bool                     `json:"no_new_privileges,omitempty"`
	OverlayVolumes     []*specgen.OverlayVolume `json:"overlay_volumes,omitempty"`
	SeccompPolicy      string                   `json:"seccomp_policy,omitempty"`
	SeccompProfilePath string                   `json:"seccomp_profile_path,omitempty"`
	SelinuxOpts        []string                 `json:"selinux_opts,omitempty"`
	Volumes            []*specgen.NamedVolume   `json:"volumes,omitempty"`
}
