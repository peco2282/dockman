package model

type SystemInfo struct {
	DockerVersion     string
	APIVersion        string
	MinAPIVersion     string
	Platform          string
	Containers        int
	Running           int
	Paused            int
	Stopped           int
	Images            int
	HostName          string
	OperatingSystem   string
	KernelVersion     string
	Architecture      string
	CPUs              int
	MemoryBytes       int64
	StorageDriver     string
	LoggingDriver     string
	CgroupDriver      string
	CgroupVersion     string
	DockerRootDir     string
	DefaultRuntime    string
	LiveRestore       bool
	ExperimentalBuild bool
}

type CleanupKind int

const (
	CleanupContainers CleanupKind = iota
	CleanupImages
	CleanupVolumes
	CleanupNetworks
	CleanupBuildCache
)

type CleanupItem struct {
	Kind CleanupKind
	ID   string
	Name string
	Size int64
}

type CleanupAnalysis struct {
	ContainersTotal int64
	ImagesTotal     int64
	VolumesTotal    int64
	BuildCacheTotal int64
	Items           []CleanupItem
}

type CleanupSelection struct {
	Containers bool
	Images     bool
	Volumes    bool
	Networks   bool
	BuildCache bool
}

type CleanupResult struct {
	Deleted        map[CleanupKind]int
	SpaceReclaimed uint64
}
