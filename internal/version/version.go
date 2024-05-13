// Copyright 2024 Humanitec
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package version

import (
	"fmt"
	"runtime/debug"
)

var (
	Version string = "0.0.0"
)

// BuildVersionString constructs a version string by looking at the build metadata injected at build time.
// This is particularly useful when score-k8s is installed from the go module using go install.
func BuildVersionString() string {
	versionNumber, buildTime, gitSha, isDirtySuffix := Version, "local", "unknown", ""
	if info, ok := debug.ReadBuildInfo(); ok {
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			versionNumber = info.Main.Version
		}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.time":
				if setting.Value != "" {
					buildTime = setting.Value
				}
			case "vcs.revision":
				if setting.Value != "" {
					gitSha = setting.Value
				}
			case "vcs.modified":
				if setting.Value == "true" {
					isDirtySuffix = "-dirty"
				}
			}
		}
	}
	return fmt.Sprintf("%s (build: %s, sha: %s%s)", versionNumber, buildTime, gitSha, isDirtySuffix)
}
