//go:build ignore

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
)

func createLayer(srcFile, destTarGz, entryName string) error {
	sf, err := os.Open(srcFile)
	if err != nil {
		return err
	}
	defer sf.Close()

	fi, err := sf.Stat()
	if err != nil {
		return err
	}

	tf, err := os.Create(destTarGz)
	if err != nil {
		return err
	}
	defer tf.Close()

	gw := gzip.NewWriter(tf)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	header := &tar.Header{
		Name:     entryName,
		Mode:     0755, // Explicit POSIX executable permission
		Size:     fi.Size(),
		Typeflag: tar.TypeReg,
		Uid:      65532,
		Gid:      65532,
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, sf)
	return err
}

func main() {
	tasks := []struct {
		src, dest, entry string
	}{
		{"bin/webhook", "layer-webhook.tar.gz", "webhook"},
		{"bin/controller", "layer-controller.tar.gz", "controller"},
		{"bin/scheduler-plugin", "layer-scheduler.tar.gz", "scheduler-plugin"},
		{"bin/simulator", "layer-simulator.tar.gz", "simulator"},
	}

	for _, t := range tasks {
		fmt.Printf("Packing %s -> %s (0755)...\n", t.src, t.dest)
		if err := createLayer(t.src, t.dest, t.entry); err != nil {
			panic(err)
		}
	}
	fmt.Println("All layers successfully packed with 0755 permissions!")
}
