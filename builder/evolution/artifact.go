// Copyright (c) 2026 the packer-plugin-cloud-evolution authors.
// SPDX-License-Identifier: Apache-2.0

package evolution

import (
	"context"
	"fmt"
	"log"
)

const BuilderID = "packer.cloud-evolution"

// Artifact is the private Evolution image produced by a successful build.
type Artifact struct {
	Image     Image
	StateData map[string]interface{}
	driver    Driver
}

func (*Artifact) BuilderId() string { return BuilderID }

func (a *Artifact) Id() string { return a.Image.ID }

func (*Artifact) Files() []string { return nil }

func (a *Artifact) String() string {
	return fmt.Sprintf("A private Evolution image was created: %s (id: %s)", a.Image.Name, a.Image.ID)
}

func (a *Artifact) State(name string) interface{} {
	if a.StateData != nil {
		if v, ok := a.StateData[name]; ok {
			return v
		}
	}
	switch name {
	case "ImageID":
		return a.Image.ID
	case "ImageName":
		return a.Image.Name
	case "ImageType":
		return a.Image.Type
	}
	return nil
}

func (a *Artifact) Destroy() error {
	if a.driver == nil || a.Image.ID == "" {
		return nil
	}
	log.Printf("Destroying Evolution image %s", a.Image.ID)
	ctx, cancel := context.WithTimeout(context.Background(), artifactDestroyTimeout)
	defer cancel()
	return a.driver.DeleteImage(ctx, a.Image.ID)
}
