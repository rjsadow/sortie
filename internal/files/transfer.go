// Package files provides file transfer operations for session workspaces.
// Files are transferred to/from running session pods using the Kubernetes
// exec API, similar to "kubectl cp".
package files

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/rjsadow/launchpad/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	// WorkspaceVolumeName is the name of the shared workspace volume
	WorkspaceVolumeName = "workspace"

	// WorkspaceMountPath is the mount path for the workspace volume in the app container
	WorkspaceMountPath = "/workspace"

	// AppContainerName is the name of the application container in the pod
	AppContainerName = "app"
)

// FileInfo represents metadata about a file in the workspace
type FileInfo struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime string `json:"mod_time,omitempty"`
}

// UploadFile writes file content to a path inside the session pod's workspace.
func UploadFile(ctx context.Context, podName, filename string, content io.Reader, size int64) error {
	// Sanitize filename to prevent path traversal
	cleanName := path.Clean(filename)
	if strings.Contains(cleanName, "..") {
		return fmt.Errorf("invalid filename: path traversal not allowed")
	}

	// Create a tar archive containing the file
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	hdr := &tar.Header{
		Name: cleanName,
		Mode: 0644,
		Size: size,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}
	if _, err := io.Copy(tw, content); err != nil {
		return fmt.Errorf("failed to write file to tar: %w", err)
	}
	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to close tar writer: %w", err)
	}

	// Extract the tar archive inside the pod's workspace directory
	cmd := []string{"tar", "xf", "-", "-C", WorkspaceMountPath}
	stdin := bytes.NewReader(tarBuf.Bytes())
	var stderr bytes.Buffer

	if err := execInPod(ctx, podName, AppContainerName, cmd, stdin, io.Discard, &stderr); err != nil {
		return fmt.Errorf("failed to upload file: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// DownloadFile reads a file from the session pod's workspace and writes it to the provided writer.
func DownloadFile(ctx context.Context, podName, filePath string, w io.Writer) error {
	// Sanitize path to prevent traversal
	cleanPath := path.Clean(filePath)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: path traversal not allowed")
	}

	fullPath := path.Join(WorkspaceMountPath, cleanPath)

	// Use tar to stream the file out of the pod
	cmd := []string{"tar", "cf", "-", "-C", path.Dir(fullPath), path.Base(fullPath)}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := execInPod(ctx, podName, AppContainerName, cmd, nil, &stdout, &stderr); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "No such file") || strings.Contains(errMsg, "not found") {
			return fmt.Errorf("file not found: %s", cleanPath)
		}
		return fmt.Errorf("failed to download file: %w (stderr: %s)", err, errMsg)
	}

	// Extract the single file from the tar archive
	tr := tar.NewReader(&stdout)
	hdr, err := tr.Next()
	if err != nil {
		return fmt.Errorf("file not found in tar archive: %s", cleanPath)
	}
	_ = hdr // We only need the content, not the header

	if _, err := io.Copy(w, tr); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// ListFiles returns a list of files in the session pod's workspace directory.
func ListFiles(ctx context.Context, podName, dirPath string) ([]FileInfo, error) {
	// Sanitize and build full path
	cleanPath := path.Clean(dirPath)
	if strings.Contains(cleanPath, "..") {
		return nil, fmt.Errorf("invalid path: path traversal not allowed")
	}

	fullPath := path.Join(WorkspaceMountPath, cleanPath)

	// List directory contents with type and size
	cmd := []string{"sh", "-c", fmt.Sprintf(
		`cd %q 2>/dev/null || exit 1; for f in *; do [ -e "$f" ] || { echo "EMPTY"; exit 0; }; if [ -d "$f" ]; then echo "d|0|$f"; else size=$(wc -c < "$f" 2>/dev/null || echo 0); echo "f|$size|$f"; fi; done`,
		fullPath,
	)}

	var stdout, stderr bytes.Buffer
	if err := execInPod(ctx, podName, AppContainerName, cmd, nil, &stdout, &stderr); err != nil {
		errMsg := stderr.String()
		if strings.Contains(errMsg, "No such file") || strings.Contains(errMsg, "exit 1") {
			return nil, fmt.Errorf("directory not found: %s", cleanPath)
		}
		return nil, fmt.Errorf("failed to list files: %w (stderr: %s)", err, errMsg)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" || output == "EMPTY" {
		return []FileInfo{}, nil
	}

	var files []FileInfo
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		var size int64
		fmt.Sscanf(parts[1], "%d", &size)

		files = append(files, FileInfo{
			Name:  parts[2],
			Size:  size,
			IsDir: parts[0] == "d",
		})
	}

	return files, nil
}

// DeleteFile removes a file from the session pod's workspace.
func DeleteFile(ctx context.Context, podName, filePath string) error {
	cleanPath := path.Clean(filePath)
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("invalid path: path traversal not allowed")
	}

	fullPath := path.Join(WorkspaceMountPath, cleanPath)

	cmd := []string{"rm", "-f", fullPath}
	var stderr bytes.Buffer

	if err := execInPod(ctx, podName, AppContainerName, cmd, nil, io.Discard, &stderr); err != nil {
		return fmt.Errorf("failed to delete file: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

// execInPod executes a command in a container within a pod.
func execInPod(ctx context.Context, podName, containerName string, cmd []string, stdin io.Reader, stdout, stderr io.Writer) error {
	client, err := k8s.GetClient()
	if err != nil {
		return fmt.Errorf("failed to get kubernetes client: %w", err)
	}

	restConfig, err := k8s.GetRESTConfig()
	if err != nil {
		return fmt.Errorf("failed to get REST config: %w", err)
	}

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(k8s.GetNamespace()).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   cmd,
			Stdin:     stdin != nil,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	return exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
}
