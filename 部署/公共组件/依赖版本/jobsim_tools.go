// Jobs · JobsIM 的构建与存储工具元数据；不安装或升级数据库。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const nodeMetadataURL = "https://nodejs.org/dist/index.json"
const postgresMetadataURL = "https://www.postgresql.org/versions.json"

type nodeRelease struct {
	Version string          `json:"version"`
	LTS     json.RawMessage `json:"lts"`
	Files   []string        `json:"files"`
}

func (v versionResolver) resolveNode(ctx context.Context, platform, arch string) (releaseRecord, error) {
	var releases []nodeRelease
	if err := v.readJSON(ctx, nodeMetadataURL, &releases); err != nil {
		return releaseRecord{}, err
	}
	var latest *nodeRelease
	var version string
	for i := range releases {
		r := &releases[i]
		candidate := strings.TrimPrefix(r.Version, "v")
		var lts string
		if !strings.HasPrefix(r.Version, "v") || json.Unmarshal(r.LTS, &lts) != nil || lts == "" {
			continue
		}
		if _, err := parseVersion(candidate); err != nil || strings.Count(candidate, ".") != 2 {
			continue
		}
		if latest == nil {
			latest, version = r, candidate
			continue
		}
		if order, _ := compareVersion(candidate, version); order > 0 {
			latest, version = r, candidate
		}
	}
	if latest == nil {
		return releaseRecord{}, errors.New("官方目录没有有效 Node.js LTS 正式发布")
	}
	targetArch := arch
	if arch == "amd64" {
		targetArch = "x64"
	}
	targetOS := platform
	if platform == "windows" {
		targetOS = "win"
	}
	extension, kind := ".tar.gz", "tar.gz"
	if platform == "windows" {
		extension, kind = ".zip", "zip"
	}
	filename := "node-v" + version + "-" + targetOS + "-" + targetArch + extension
	fileType := targetOS + "-" + targetArch
	if platform == "darwin" {
		fileType = "osx-" + targetArch + "-tar"
	}
	if platform == "windows" {
		fileType += "-zip"
	}
	found := false
	for _, item := range latest.Files {
		if item == fileType {
			found = true
		}
	}
	if !found {
		return unsupported("node", platform, arch, version), nil
	}
	url := "https://nodejs.org/dist/v" + version + "/"
	sums, err := v.readMetadata(ctx, url+"SHASUMS256.txt")
	if err != nil {
		return releaseRecord{}, err
	}
	digest := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == filename {
			if digest != "" || !shaPattern.MatchString(fields[0]) {
				return releaseRecord{}, errors.New("Node.js 归档摘要无效或重复")
			}
			digest = strings.ToLower(fields[0])
		}
	}
	if digest == "" {
		return releaseRecord{}, errors.New("Node.js 官方摘要清单缺少目标归档")
	}
	return releaseRecord{"node", platform, arch, version, filename, digest, url + filename, kind}, nil
}

type postgresRelease struct {
	Major       string `json:"major"`
	LatestMinor string `json:"latestMinor"`
	Supported   bool   `json:"supported"`
	Current     bool   `json:"current"`
}

func (v versionResolver) resolvePostgres(ctx context.Context, platform, arch string) (releaseRecord, error) {
	var releases []postgresRelease
	if err := v.readJSON(ctx, postgresMetadataURL, &releases); err != nil {
		return releaseRecord{}, err
	}
	version := ""
	for _, r := range releases {
		if !r.Supported || !r.Current {
			continue
		}
		candidate := r.Major + "." + r.LatestMinor
		if _, err := parseVersion(candidate); err != nil || strings.Count(candidate, ".") != 1 {
			return releaseRecord{}, fmt.Errorf("PostgreSQL 官方当前版本无效")
		}
		if version != "" {
			return releaseRecord{}, errors.New("PostgreSQL 官方目录含多个当前版本")
		}
		version = candidate
	}
	if version == "" {
		return releaseRecord{}, errors.New("PostgreSQL 官方目录没有当前受支持版本")
	}
	// 官方版本目录不提供通用二进制资产；原生入口检查包源与兼容主版本，禁止拿源码包冒充原生SDK。
	return unsupported("postgres", platform, arch, version), nil
}
