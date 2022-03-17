package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/spf13/cobra"

	"github.com/operator-framework/rukpak/internal/version"
)

func main() {
	var (
		bundleDir  string
		listenAddr string

		rukpakVersion bool
	)

	cmd := &cobra.Command{
		Use:  "unpack",
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if rukpakVersion {
				fmt.Printf("Git commit: %s\n", version.String())
				os.Exit(0)
			}
			contentHandler := serveBundle(os.DirFS(bundleDir))
			h := mux.NewRouter()
			h.Handle("/content", contentHandler).Methods(http.MethodGet)
			srv := http.Server{
				Addr:         listenAddr,
				Handler:      handlers.CombinedLoggingHandler(os.Stdout, h),
				ReadTimeout:  1 * time.Second,
				WriteTimeout: 10 * time.Second,
			}
			go func() {
				log.Println("listening on", srv.Addr)
				if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Fatal(err)
				}
			}()

			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			<-ctx.Done()

			shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelShutdown()
			log.Println("shutting down")
			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Fatal(err)
			}
			log.Println("shutdown complete")

			return nil
		},
	}
	cmd.Flags().StringVar(&bundleDir, "bundle-dir", "", "directory in which the bundle can be found")
	cmd.Flags().StringVar(&listenAddr, "listen-addr", ":8080", "listen address for http server")
	cmd.Flags().BoolVar(&rukpakVersion, "version", false, "displays rukpak version information")

	if err := cmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func serveBundle(bundle fs.FS) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		buf := &bytes.Buffer{}
		gzw := gzip.NewWriter(buf)
		tw := tar.NewWriter(gzw)
		if err := fs.WalkDir(bundle, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if d.Type()&os.ModeSymlink != 0 {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("get file info for %q: %w", path, err)
			}

			h, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return fmt.Errorf("build tar file info header for %q: %w", path, err)
			}
			h.Uid = 0
			h.Gid = 0
			h.Uname = ""
			h.Gname = ""
			h.Name = path

			if err := tw.WriteHeader(h); err != nil {
				return fmt.Errorf("write tar header for %q: %w", path, err)
			}
			if !d.IsDir() {
				data, err := fs.ReadFile(bundle, path)
				if err != nil {
					return fmt.Errorf("read file %q: %w", path, err)
				}
				if _, err := tw.Write(data); err != nil {
					return fmt.Errorf("write tar data for %q: %w", path, err)
				}
			}
			return nil
		}); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		if err := tw.Close(); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		if err := gzw.Close(); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
		if _, err := io.Copy(writer, buf); err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
