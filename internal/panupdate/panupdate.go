// Copyright (c) 2020 tickstep.
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
package panupdate

import (
	"archive/zip"
	"bytes"
	"fmt"
	"github.com/phpc0de/ctpango/cmder/cmdliner"
	"github.com/phpc0de/ctpango/cmder/cmdutil"
	"github.com/phpc0de/ctpango/internal/config"
	"github.com/phpc0de/ctpango/internal/utils"
	"github.com/phpc0de/ctlibgo/cachepool"
	"github.com/phpc0de/ctlibgo/checkaccess"
	"github.com/phpc0de/ctlibgo/converter"
	"github.com/phpc0de/ctlibgo/getip"
	"github.com/phpc0de/ctlibgo/jsonhelper"
	"github.com/phpc0de/ctlibgo/logger"
	"github.com/phpc0de/ctlibgo/requester"
	"github.com/phpc0de/ctpango/library/requester/transfer"
	"net/http"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	ReleaseName = "cloudpan189-go"
)

type info struct {
	filename    string
	size        int64
	downloadURL string
}

type tsResp struct {
	Code int `json:"code"`
	Data interface{} `json:"data"`
	Msg string `json:"msg"`
}

func getReleaseFromTicstep(client *requester.HTTPClient, showPrompt bool) *ReleaseInfo {
	tsReleaseInfo := &ReleaseInfo{}
	tsResp := &tsResp{Data: tsReleaseInfo}
	fullUrl := strings.Builder{}
	ipAddr, err := getip.IPInfoFromTechainBaidu()
	if err != nil {
		ipAddr = "127.0.0.1"
	}
	fmt.Fprintf(&fullUrl, "http://api.tickstep.com/update/tickstep/cloudpan189-go/releases/latest?ip=%s&os=%s&arch=%s&version=%s",
		ipAddr, runtime.GOOS, runtime.GOARCH, config.AppVersion)
	resp, err := client.Req(http.MethodGet, fullUrl.String(), nil, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if showPrompt {
			logger.Verbosef("??????????????????: %s\n", err)
		}
		return nil
	}
	err = jsonhelper.UnmarshalData(resp.Body, tsResp)
	if err != nil {
		if showPrompt {
			fmt.Printf("json??????????????????: %s\n", err)
		}
		return nil
	}
	if tsResp.Code == 0 {
		return tsReleaseInfo
	}
	return nil
}

func getReleaseFromGithub(client *requester.HTTPClient, showPrompt bool) *ReleaseInfo {
	resp, err := client.Req(http.MethodGet, "https://api.github.com/repos/tickstep/cloudpan189-go/releases/latest", nil, nil)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if showPrompt {
			logger.Verbosef("??????????????????: %s\n", err)
		}
		return nil
	}

	releaseInfo := ReleaseInfo{}
	err = jsonhelper.UnmarshalData(resp.Body, &releaseInfo)
	if err != nil {
		if showPrompt {
			fmt.Printf("json??????????????????: %s\n", err)
		}
		return nil
	}
	return &releaseInfo
}

func GetLatestReleaseInfo(showPrompt bool) *ReleaseInfo {
	client := config.Config.HTTPClient("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36")
	client.SetTimeout(time.Duration(0) * time.Second)
	client.SetKeepAlive(true)

	// check tickstep srv
	var tsReleaseInfo *ReleaseInfo = nil
	for idx := 0; idx < 3; idx++ {
		tsReleaseInfo = getReleaseFromTicstep(client, showPrompt)
		if tsReleaseInfo != nil {
			break
		}
		time.Sleep(time.Duration(5) * time.Second)
	}

	// github
	var ghReleaseInfo *ReleaseInfo = nil
	for idx := 0; idx < 3; idx++ {
		ghReleaseInfo = getReleaseFromGithub(client, showPrompt)
		if ghReleaseInfo != nil {
			break
		}
		time.Sleep(time.Duration(5) * time.Second)
	}

	var releaseInfo *ReleaseInfo = nil
	if config.Config.UpdateCheckInfo.PreferUpdateSrv == "tickstep" {
		// theoretically, tickstep server will be more faster at mainland
		releaseInfo = tsReleaseInfo
	} else {
		releaseInfo = ghReleaseInfo
		if ghReleaseInfo == nil {
			releaseInfo = tsReleaseInfo
		}
	}
	return releaseInfo
}

// CheckUpdate ????????????
func CheckUpdate(version string, yes bool) {
	if !checkaccess.AccessRDWR(cmdutil.ExecutablePath()) {
		fmt.Printf("?????????????????????, ????????????.\n")
		return
	}
	fmt.Println("???????????????, ??????...")
	client := config.Config.HTTPClient("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36")
	client.SetTimeout(time.Duration(0) * time.Second)
	client.SetKeepAlive(true)

	releaseInfo := GetLatestReleaseInfo(true)
	if releaseInfo == nil {
		fmt.Printf("????????????????????????!\n")
		return
	}

	// ????????????, ????????? Beta ??????, ????????????????????????
	if strings.Contains(releaseInfo.TagName, "Beta") || !strings.HasPrefix(releaseInfo.TagName, "v") || utils.ParseVersionNum(version) >= utils.ParseVersionNum(releaseInfo.TagName) {
		fmt.Printf("??????????????????!\n")
		return
	}

	fmt.Printf("??????????????????: %s\n", releaseInfo.TagName)

	line := cmdliner.NewLiner()
	defer line.Close()

	if !yes {
		y, err := line.State.Prompt("?????????????????? (y/n): ")
		if err != nil {
			fmt.Printf("????????????: %s\n", err)
			return
		}

		if y != "y" && y != "Y" {
			fmt.Printf("????????????.\n")
			return
		}
	}

	builder := &strings.Builder{}
	builder.WriteString(ReleaseName + "-" + releaseInfo.TagName + "-" + runtime.GOOS + "-.*?")
	if runtime.GOOS == "darwin" && (runtime.GOARCH == "arm" || runtime.GOARCH == "arm64") {
		builder.WriteString("arm")
	} else {
		switch runtime.GOARCH {
		case "amd64":
			builder.WriteString("(amd64|x86_64|x64)")
		case "386":
			builder.WriteString("(386|x86)")
		case "arm":
			builder.WriteString("(armv5|armv7|arm)")
		case "arm64":
			builder.WriteString("arm64")
		case "mips":
			builder.WriteString("mips")
		case "mips64":
			builder.WriteString("mips64")
		case "mipsle":
			builder.WriteString("(mipsle|mipsel)")
		case "mips64le":
			builder.WriteString("(mips64le|mips64el)")
		default:
			builder.WriteString(runtime.GOARCH)
		}
	}
	builder.WriteString("\\.zip")

	exp := regexp.MustCompile(builder.String())

	var targetList []*info
	for _, asset := range releaseInfo.Assets {
		if asset == nil || asset.State != "uploaded" {
			continue
		}

		if exp.MatchString(asset.Name) {
			targetList = append(targetList, &info{
				filename:    asset.Name,
				size:        asset.Size,
				downloadURL: asset.BrowserDownloadURL,
			})
		}
	}

	var target info
	switch len(targetList) {
	case 0:
		fmt.Printf("?????????????????????????????????????????????, GOOS: %s, GOARCH: %s\n", runtime.GOOS, runtime.GOARCH)
		return
	case 1:
		target = *targetList[0]
	default:
		fmt.Println()
		for k := range targetList {
			fmt.Printf("%d: %s\n", k, targetList[k].filename)
		}

		fmt.Println()
		t, err := line.State.Prompt("???????????????????????????: ")
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}

		i, err := strconv.Atoi(t)
		if err != nil {
			fmt.Printf("????????????: %s\n", err)
			return
		}

		if i < 0 || i >= len(targetList) {
			fmt.Printf("????????????: ?????????????????????\n")
			return
		}

		target = *targetList[i]
	}

	if target.size > 0x7fffffff {
		fmt.Printf("file size too large: %d\n", target.size)
		return
	}

	fmt.Printf("??????????????????: %s\n", target.filename)

	// ????????????
	buf := cachepool.RawMallocByteSlice(int(target.size))
	resp, err := client.Req("GET", target.downloadURL, nil, nil)
	if err != nil {
		fmt.Printf("??????????????????????????????: %s\n", err)
		return
	}
	total, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	if total > 0 {
		if int64(total) != target.size {
			fmt.Printf("??????????????????????????????: %s\n", err)
			return
		}
	}

	// ???????????????
	var readErr error
	downloadSize := 0
	nn := 0
	nn64 := int64(0)
	downloadStatus := transfer.NewDownloadStatus()
	downloadStatus.AddTotalSize(target.size)

	statusIndicator := func(status *transfer.DownloadStatus) {
		status.UpdateSpeeds() // ????????????
		var leftStr string
		left := status.TimeLeft()
		if left < 0 {
			leftStr = "-"
		} else {
			leftStr = left.String()
		}

		fmt.Printf("\r ??? %s/%s %s/s in %s, left %s ............",
			converter.ConvertFileSize(status.Downloaded(), 2),
			converter.ConvertFileSize(status.TotalSize(), 2),
			converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
			status.TimeElapsed()/1e7*1e7, leftStr,
		)
	}

	// ????????????
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for downloadSize < len(buf) && readErr == nil {
			nn, readErr = resp.Body.Read(buf[downloadSize:])
			nn64 = int64(nn)

			// ??????????????????
			downloadStatus.AddSpeedsDownloaded(nn64)
			downloadStatus.AddDownloaded(nn64)
			downloadSize += nn

			if statusIndicator != nil {
				statusIndicator(downloadStatus)
			}
		}
	}()
	wg.Wait()

	if int64(downloadSize) == target.size {
		// ????????????
		fmt.Printf("\n????????????\n")
	} else {
		fmt.Printf("\n????????????????????????\n")
		return
	}

	// ????????????
	reader, err := zip.NewReader(bytes.NewReader(buf), target.size)
	if err != nil {
		fmt.Printf("??????????????????????????????: %s\n", err)
		return
	}

	execPath := cmdutil.ExecutablePath()

	var fileNum, errTimes int
	for _, zipFile := range reader.File {
		if zipFile == nil {
			continue
		}

		info := zipFile.FileInfo()

		if info.IsDir() {
			continue
		}

		rc, err := zipFile.Open()
		if err != nil {
			fmt.Printf("?????? zip ????????????: %s\n", err)
			continue
		}

		fileNum++

		name := zipFile.Name[strings.Index(zipFile.Name, "/")+1:]
		if name == ReleaseName {
			err = update(cmdutil.Executable(), rc)
		} else {
			err = update(filepath.Join(execPath, name), rc)
		}

		if err != nil {
			errTimes++
			fmt.Printf("????????????, zip ??????: %s, ??????: %s\n", zipFile.Name, err)
			continue
		}
	}

	if errTimes == fileNum {
		fmt.Printf("????????????\n")
		return
	}

	fmt.Printf("????????????, ???????????????\n")
}
