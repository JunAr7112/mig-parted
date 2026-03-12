/*
 * Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package discover

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v2"

	nvdev "github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/mig-parted/pkg/mig/discovery"
)

var log = logrus.New()

func GetLogger() *logrus.Logger {
	return log
}

const (
	JSONFormat = "json"
	YAMLFormat = "yaml"
)

type Flags struct {
	OutputFile   string
	OutputFormat string
}

// profileInfoOut is the serializable form of discovery.ProfileInfo for JSON/YAML output.
type profileInfoOut struct {
	Name     string            `json:"name"      yaml:"name"`
	MaxCount int               `json:"max_count" yaml:"max_count"`
	Info     *migProfileInfoOut `json:"info,omitempty" yaml:"info,omitempty"`
}

// migProfileInfoOut is the serializable form of go-nvlib MigProfileInfo (C, G, GB, NVML profile IDs, etc.).
type migProfileInfoOut struct {
	C              int      `json:"c"               yaml:"c"`
	G              int      `json:"g"               yaml:"g"`
	GB             int      `json:"gb"              yaml:"gb"`
	Attributes     []string `json:"attributes,omitempty"     yaml:"attributes,omitempty"`
	NegAttributes  []string `json:"neg_attributes,omitempty" yaml:"neg_attributes,omitempty"`
	GIProfileID    int      `json:"gi_profile_id"   yaml:"gi_profile_id"`
	CIProfileID    int      `json:"ci_profile_id"   yaml:"ci_profile_id"`
	CIEngProfileID int      `json:"ci_eng_profile_id" yaml:"ci_eng_profile_id"`
}

// deviceProfilesOut is the serializable form of discovery.DeviceProfiles for JSON/YAML output.
type deviceProfilesOut struct {
	Devices []deviceProfilesEntry `json:"devices" yaml:"devices"`
}

type deviceProfilesEntry struct {
	DeviceIndex int             `json:"device_index" yaml:"device_index"`
	DeviceID    string          `json:"device_id"    yaml:"device_id"`
	Profiles    []profileInfoOut `json:"profiles"    yaml:"profiles"`
}

func BuildCommand() *cli.Command {
	discoverFlags := Flags{}

	discover := cli.Command{}
	discover.Name = "discover"
	discover.Usage = "Discover MIG profiles supported by each GPU (profile name, max count, device ID)"
	discover.Action = func(c *cli.Context) error {
		return runDiscover(c, &discoverFlags)
	}

	discover.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:        "output-file",
			Aliases:     []string{"o"},
			Usage:       "Output file path (default: stdout)",
			Destination: &discoverFlags.OutputFile,
			Value:       "",
			EnvVars:     []string{"MIG_PARTED_DISCOVER_OUTPUT_FILE"},
		},
		&cli.StringFlag{
			Name:        "output-format",
			Aliases:     []string{"f"},
			Usage:       "Output format [json | yaml]",
			Destination: &discoverFlags.OutputFormat,
			Value:       YAMLFormat,
			EnvVars:     []string{"MIG_PARTED_DISCOVER_OUTPUT_FORMAT"},
		},
	}

	return &discover
}

func runDiscover(c *cli.Context, f *Flags) error {
	if f.OutputFormat != JSONFormat && f.OutputFormat != YAMLFormat {
		return fmt.Errorf("unrecognized output format: %s (use json or yaml)", f.OutputFormat)
	}

	deviceProfiles, err := discovery.DiscoverMIGProfiles()
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	out := deviceProfilesToOut(deviceProfiles)

	writer := io.Writer(os.Stdout)
	if f.OutputFile != "" {
		file, err := os.Create(f.OutputFile)
		if err != nil {
			return fmt.Errorf("error creating output file: %w", err)
		}
		defer file.Close()
		writer = file
	}

	var payload []byte
	switch f.OutputFormat {
	case JSONFormat:
		payload, err = json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
	case YAMLFormat:
		payload, err = yaml.Marshal(out)
		if err != nil {
			return fmt.Errorf("error marshaling YAML: %w", err)
		}
	}

	if _, err := writer.Write(payload); err != nil {
		return fmt.Errorf("error writing output: %w", err)
	}

	return nil
}

func deviceProfilesToOut(dp discovery.DeviceProfiles) deviceProfilesOut {
	// Sort device indices for stable output
	indices := make([]int, 0, len(dp))
	for i := range dp {
		indices = append(indices, i)
	}
	sort.Ints(indices)

	entries := make([]deviceProfilesEntry, 0, len(indices))
	for _, i := range indices {
		profiles := dp[i]
		if len(profiles) == 0 {
			continue
		}
		deviceID := profiles[0].DeviceID.String()
		profilesOut := make([]profileInfoOut, len(profiles))
		for j, p := range profiles {
			po := profileInfoOut{Name: p.Name, MaxCount: p.MaxCount}
			if p.Profile != nil {
				info := p.Profile.GetInfo()
				po.Info = migProfileInfoFromNVLib(info)
			}
			profilesOut[j] = po
		}
		entries = append(entries, deviceProfilesEntry{
			DeviceIndex: i,
			DeviceID:    deviceID,
			Profiles:    profilesOut,
		})
	}
	return deviceProfilesOut{Devices: entries}
}

func migProfileInfoFromNVLib(info nvdev.MigProfileInfo) *migProfileInfoOut {
	out := &migProfileInfoOut{
		C:              info.C,
		G:              info.G,
		GB:             info.GB,
		GIProfileID:    info.GIProfileID,
		CIProfileID:    info.CIProfileID,
		CIEngProfileID: info.CIEngProfileID,
	}
	if len(info.Attributes) > 0 {
		out.Attributes = make([]string, len(info.Attributes))
		copy(out.Attributes, info.Attributes)
	}
	if len(info.NegAttributes) > 0 {
		out.NegAttributes = make([]string, len(info.NegAttributes))
		copy(out.NegAttributes, info.NegAttributes)
	}
	return out
}
