package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/lmccay/volumez/internal/pathmap"
	"github.com/lmccay/volumez/pkg/backend"
	_ "github.com/lmccay/volumez/pkg/backend/http" // Register HTTP backend
	_ "github.com/lmccay/volumez/pkg/backend/s3"   // Register S3 backend
	"github.com/lmccay/volumez/pkg/config"
	vfs "github.com/lmccay/volumez/pkg/fuse"
)

var (
	configPath  = flag.String("config", "volumez.json", "Path to configuration file")
	mountPoint  = flag.String("mount", "", "Mount point (required)")
	genConfig   = flag.Bool("gen-config", false, "Generate example configuration and exit")
	debug       = flag.Bool("debug", false, "Enable debug output")
	allowOther  = flag.Bool("allow-other", false, "Allow other users to access the filesystem")
	allowRoot   = flag.Bool("allow-root", false, "Allow root to access the filesystem")
	readOnly    = flag.Bool("read-only", false, "Mount filesystem as read-only")
	showVersion = flag.Bool("version", false, "Show version information")
)

const version = "0.1.0"

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Printf("volumez v%s\n", version)
		os.Exit(0)
	}

	if *genConfig {
		if err := generateConfig(); err != nil {
			log.Fatalf("Failed to generate config: %v", err)
		}
		fmt.Println("Example configuration written to volumez.example.json")
		os.Exit(0)
	}

	if *mountPoint == "" {
		log.Fatal("Mount point is required. Use -mount flag.")
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if *debug {
		cfg.Debug = true
	}

	// Create path mapper and initialize backends
	mapper := pathmap.New()

	for _, mountCfg := range cfg.Mounts {
		log.Printf("Initializing backend %s for path %s", mountCfg.Backend, mountCfg.Path)

		b, err := backend.Create(mountCfg.Backend, mountCfg.Config)
		if err != nil {
			log.Fatalf("Failed to create backend %s: %v", mountCfg.Backend, err)
		}

		if err := mapper.AddMount(mountCfg.Path, b); err != nil {
			log.Fatalf("Failed to add mount %s: %v", mountCfg.Path, err)
		}
	}

	// Create FUSE filesystem root
	root := vfs.NewFS(mapper, cfg.Debug)

	// Mount options
	attrTimeout := time.Second * 60
	entryTimeout := time.Second * 60
	opts := &fs.Options{
		MountOptions: fuse.MountOptions{
			Name:          "volumez",
			FsName:        "volumez",
			DisableXAttrs: true,
		},
		AttrTimeout:  &attrTimeout,   // Cache attributes for 60 seconds
		EntryTimeout: &entryTimeout,  // Cache directory entries for 60 seconds
	}

	if *allowOther {
		opts.MountOptions.AllowOther = true
	}

	if *debug {
		opts.MountOptions.Debug = true
	}

	// Mount the filesystem
	log.Printf("Mounting filesystem at %s", *mountPoint)
	server, err := fs.Mount(*mountPoint, root, opts)
	if err != nil {
		log.Fatalf("Failed to mount: %v", err)
	}

	log.Printf("Filesystem mounted successfully. Press Ctrl+C to unmount.")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	<-sigChan
	log.Println("Received interrupt signal, unmounting...")

	// Unmount
	if err := server.Unmount(); err != nil {
		log.Printf("Error unmounting: %v", err)
	}

	// Wait for server to finish
	server.Wait()

	// Give it a moment to clean up
	time.Sleep(100 * time.Millisecond)

	// Close all backends
	if err := mapper.Close(); err != nil {
		log.Printf("Error closing backends: %v", err)
	}

	log.Println("Filesystem unmounted successfully")
}

func generateConfig() error {
	cfg := config.Example()
	return cfg.Save("volumez.example.json")
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}
