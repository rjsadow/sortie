package k8s

import (
	"testing"
)

func TestBuildPodSpec_HasWorkspaceVolume(t *testing.T) {
	config := DefaultPodConfig("sess-1", "app-1", "Test App", "ubuntu:latest")

	pod := BuildPodSpec(config)

	// Check that workspace volume exists
	foundVolume := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == WorkspaceVolumeName {
			foundVolume = true
			if v.EmptyDir == nil {
				t.Error("workspace volume should use EmptyDir")
			}
			break
		}
	}
	if !foundVolume {
		t.Error("workspace volume not found in pod spec")
	}

	// Check that app container has workspace mount
	for _, c := range pod.Spec.Containers {
		if c.Name == "app" {
			foundMount := false
			for _, vm := range c.VolumeMounts {
				if vm.Name == WorkspaceVolumeName && vm.MountPath == WorkspaceMountPath {
					foundMount = true
					break
				}
			}
			if !foundMount {
				t.Errorf("app container missing workspace volume mount at %s", WorkspaceMountPath)
			}
		}
	}
}

func TestBuildPodSpec_JlesageHasWorkspaceVolume(t *testing.T) {
	config := DefaultPodConfig("sess-1", "app-1", "JLesage App", "jlesage/firefox:latest")

	pod := BuildPodSpec(config)

	// Check that workspace volume exists
	foundVolume := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == WorkspaceVolumeName {
			foundVolume = true
			break
		}
	}
	if !foundVolume {
		t.Error("workspace volume not found in jlesage pod spec")
	}

	// Check app container mount
	for _, c := range pod.Spec.Containers {
		if c.Name == "app" {
			foundMount := false
			for _, vm := range c.VolumeMounts {
				if vm.Name == WorkspaceVolumeName && vm.MountPath == WorkspaceMountPath {
					foundMount = true
					break
				}
			}
			if !foundMount {
				t.Errorf("jlesage app container missing workspace volume mount at %s", WorkspaceMountPath)
			}
		}
	}
}

func TestBuildWebProxyPodSpec_HasWorkspaceVolume(t *testing.T) {
	config := DefaultPodConfig("sess-1", "app-1", "Web App", "myapp:latest")
	config.ContainerPort = 8080

	pod := BuildWebProxyPodSpec(config)

	// Check that workspace volume exists
	foundVolume := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == WorkspaceVolumeName {
			foundVolume = true
			if v.EmptyDir == nil {
				t.Error("workspace volume should use EmptyDir")
			}
			break
		}
	}
	if !foundVolume {
		t.Error("workspace volume not found in web proxy pod spec")
	}

	// Check app container mount
	for _, c := range pod.Spec.Containers {
		if c.Name == "app" {
			foundMount := false
			for _, vm := range c.VolumeMounts {
				if vm.Name == WorkspaceVolumeName && vm.MountPath == WorkspaceMountPath {
					foundMount = true
					break
				}
			}
			if !foundMount {
				t.Errorf("web proxy app container missing workspace volume mount at %s", WorkspaceMountPath)
			}
		}
	}
}

func TestBuildWindowsPodSpec_HasWorkspaceVolume(t *testing.T) {
	config := DefaultPodConfig("sess-1", "app-1", "Win App", "windows:latest")

	pod := BuildWindowsPodSpec(config)

	// Check that workspace volume exists
	foundVolume := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == WorkspaceVolumeName {
			foundVolume = true
			if v.EmptyDir == nil {
				t.Error("workspace volume should use EmptyDir")
			}
			break
		}
	}
	if !foundVolume {
		t.Error("workspace volume not found in Windows pod spec")
	}

	// Check app container mount
	for _, c := range pod.Spec.Containers {
		if c.Name == "app" {
			foundMount := false
			for _, vm := range c.VolumeMounts {
				if vm.Name == WorkspaceVolumeName && vm.MountPath == WorkspaceMountPath {
					foundMount = true
					break
				}
			}
			if !foundMount {
				t.Errorf("Windows app container missing workspace volume mount at %s", WorkspaceMountPath)
			}
		}
	}
}
