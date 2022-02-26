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
package pandownload

import (
	"errors"
	"fmt"
	"github.com/phpc0de/ctapi/cloudpan"
	"github.com/phpc0de/ctapi/cloudpan/apierror"
	"github.com/phpc0de/ctpango/cmder/cmdtable"
	"github.com/phpc0de/ctpango/internal/file/downloader"
	"github.com/phpc0de/ctpango/internal/functions"
	"github.com/phpc0de/ctpango/internal/taskframework"
	"github.com/phpc0de/ctlibgo/converter"
	"github.com/phpc0de/ctlibgo/logger"
	"github.com/phpc0de/ctlibgo/requester"
	"github.com/phpc0de/ctpango/library/requester/transfer"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type (
	// DownloadTaskUnit 下载的任务单元
	DownloadTaskUnit struct {
		taskInfo *taskframework.TaskInfo // 任务信息

		Cfg                *downloader.Config
		PanClient          *cloudpan.PanClient
		ParentTaskExecutor *taskframework.TaskExecutor

		DownloadStatistic *DownloadStatistic // 下载统计

		// 可选项
		VerbosePrinter       *logger.CmdVerbose
		PrintFormat          string
		IsPrintStatus        bool // 是否输出各个下载线程的详细信息
		IsExecutedPermission bool // 下载成功后是否加上执行权限
		IsOverwrite          bool // 是否覆盖已存在的文件
		NoCheck              bool // 不校验文件

		FilePanPath string // 要下载的网盘文件路径
		SavePath    string // 文件保存在本地的路径
		OriginSaveRootPath    string // 文件保存在本地的根目录路径
		FamilyId    int64 // 家庭云ID, 个人云默认为0

		fileInfo *cloudpan.AppFileEntity // 文件或目录详情
	}
)

const (
	// DefaultPrintFormat 默认的下载进度输出格式
	DefaultPrintFormat = "\r[%s] ↓ %s/%s %s/s in %s, left %s ............"
	//DownloadSuffix 文件下载后缀
	DownloadSuffix = ".cloudpan189-downloading"
	//StrDownloadInitError 初始化下载发生错误
	StrDownloadInitError = "初始化下载发生错误"
	// StrDownloadFailed 下载文件失败
	StrDownloadFailed = "下载文件失败"
	// StrDownloadGetDlinkFailed 获取下载链接失败
	StrDownloadGetDlinkFailed = "获取下载链接失败"
	// StrDownloadChecksumFailed 检测文件有效性失败
	StrDownloadChecksumFailed = "检测文件有效性失败"
	// DefaultDownloadMaxRetry 默认下载失败最大重试次数
	DefaultDownloadMaxRetry = 3
)

func (dtu *DownloadTaskUnit) SetTaskInfo(info *taskframework.TaskInfo) {
	dtu.taskInfo = info
}

func (dtu *DownloadTaskUnit) verboseInfof(format string, a ...interface{}) {
	if dtu.VerbosePrinter != nil {
		dtu.VerbosePrinter.Infof(format, a...)
	}
}

// download 执行下载
func (dtu *DownloadTaskUnit) download() (err error) {
	var (
		writer downloader.Writer
		file   *os.File
	)

	dtu.Cfg.InstanceStatePath = dtu.SavePath + DownloadSuffix

	// 创建下载的目录
	// 获取SavePath所在的目录
	dir := filepath.Dir(dtu.SavePath)
	fileInfo, err := os.Stat(dir)
	if err != nil {
		// 目录不存在, 创建
		err = os.MkdirAll(dir, 0777)
		if err != nil {
			return err
		}
	} else if !fileInfo.IsDir() {
		// SavePath所在的目录不是目录
		return fmt.Errorf("%s, path %s: not a directory", StrDownloadInitError, dir)
	}

	// 打开文件
	writer, file, err = downloader.NewDownloaderWriterByFilename(dtu.SavePath, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		return fmt.Errorf("%s, %s", StrDownloadInitError, err)
	}
	defer file.Close()

	der := downloader.NewDownloader(writer, dtu.Cfg, dtu.PanClient)
	der.SetFileInfo(dtu.fileInfo)
	der.SetFamilyId(dtu.FamilyId)
	der.SetStatusCodeBodyCheckFunc(func(respBody io.Reader) error {
		// 解析错误
		return apierror.NewFailedApiError("")
	})

	// 检查输出格式
	if dtu.PrintFormat == "" {
		dtu.PrintFormat = DefaultPrintFormat
	}

	// 这里用共享变量的方式
	isComplete := false
	der.OnDownloadStatusEvent(func(status transfer.DownloadStatuser, workersCallback func(downloader.RangeWorkerFunc)) {
		// 这里可能会下载结束了, 还会输出内容
		builder := &strings.Builder{}
		if dtu.IsPrintStatus {
			// 输出所有的worker状态
			var (
				tb      = cmdtable.NewTable(builder)
			)
			tb.SetHeader([]string{"#", "status", "range", "left", "speeds", "error"})
			workersCallback(func(key int, worker *downloader.Worker) bool {
				wrange := worker.GetRange()
				tb.Append([]string{fmt.Sprint(worker.ID()), worker.GetStatus().StatusText(), wrange.ShowDetails(), strconv.FormatInt(wrange.Len(), 10), strconv.FormatInt(worker.GetSpeedsPerSecond(), 10), fmt.Sprint(worker.Err())})
				return true
			})

			// 先空两行
			builder.WriteString("\n\n")
			tb.Render()
		}

		// 如果下载速度为0, 剩余下载时间未知, 则用 - 代替
		var leftStr string
		left := status.TimeLeft()
		if left < 0 {
			leftStr = "-"
		} else {
			leftStr = left.String()
		}

		if dtu.Cfg.ShowProgress {
			fmt.Fprintf(builder, dtu.PrintFormat, dtu.taskInfo.Id(),
				converter.ConvertFileSize(status.Downloaded(), 2),
				converter.ConvertFileSize(status.TotalSize(), 2),
				converter.ConvertFileSize(status.SpeedsPerSecond(), 2),
				status.TimeElapsed()/1e7*1e7, leftStr,
			)
		}

		if !isComplete {
			// 如果未完成下载, 就输出
			fmt.Print(builder.String())
		}
	})

	der.OnExecute(func() {
		fmt.Printf("[%s] 下载开始\n\n", dtu.taskInfo.Id())
	})

	err = der.Execute()
	isComplete = true
	fmt.Print("\n")

	if err != nil {
		// check zero size file
		if err == downloader.ErrNoWokers && dtu.fileInfo.FileSize == 0 {
			// success for 0 size file
			dtu.verboseInfof("download success for zero size file")
		} else {
			// 下载发生错误
			// 下载失败, 删去空文件
			if info, infoErr := file.Stat(); infoErr == nil {
				if info.Size() == 0 {
					// 空文件, 应该删除
					dtu.verboseInfof("[%s] remove empty file: %s\n", dtu.taskInfo.Id(), dtu.SavePath)
					removeErr := os.Remove(dtu.SavePath)
					if removeErr != nil {
						dtu.verboseInfof("[%s] remove file error: %s\n", dtu.taskInfo.Id(), removeErr)
					}
				}
			}
			return err
		}
	}

	// 下载成功
	if dtu.IsExecutedPermission {
		err = file.Chmod(0766)
		if err != nil {
			fmt.Printf("[%s] 警告, 加执行权限错误: %s\n", dtu.taskInfo.Id(), err)
		}
	}
	fmt.Printf("[%s] 下载完成, 保存位置: %s\n", dtu.taskInfo.Id(), dtu.SavePath)

	return nil
}

//panHTTPClient 获取包含特定User-Agent的HTTPClient
func (dtu *DownloadTaskUnit) panHTTPClient() (client *requester.HTTPClient) {
	client = requester.NewHTTPClient()
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	client.SetTimeout(20 * time.Minute)
	client.SetKeepAlive(true)
	return client
}

func (dtu *DownloadTaskUnit) handleError(result *taskframework.TaskUnitRunResult) {
	switch value := result.Err.(type) {
	case *apierror.ApiError:
		switch value.ErrCode() {
		case apierror.ApiCodeFileNotFoundCode:
			result.NeedRetry = false
			break
		default:
			result.NeedRetry = true
		}
	case *os.PathError:
		// 系统级别的错误, 可能是权限问题
		result.NeedRetry = false
	default:
		// 其他错误, 需要重试
		result.NeedRetry = true
	}
}

//checkFileValid 检测文件有效性
func (dtu *DownloadTaskUnit) checkFileValid(result *taskframework.TaskUnitRunResult) (ok bool) {
	if dtu.NoCheck {
		// 不检测文件有效性
		return
	}

	if dtu.fileInfo.FileSize >= 128*converter.MB {
		// 大文件, 输出一句提示消息
		fmt.Printf("[%s] 开始检验文件有效性, 请稍候...\n", dtu.taskInfo.Id())
	}

	// 就在这里处理校验出错
	err := CheckFileValid(dtu.SavePath, dtu.fileInfo)
	if err != nil {
		result.ResultMessage = StrDownloadChecksumFailed
		result.Err = err
		switch err {
		case ErrDownloadNotSupportChecksum:
			// 文件不支持校验
			result.ResultMessage = "检验文件有效性"
			result.Err = err
			fmt.Printf("[%s] 检验文件有效性: %s\n", dtu.taskInfo.Id(), err)
			return true
		case ErrDownloadFileBanned:
			// 违规文件
			result.NeedRetry = false
			return
		case ErrDownloadChecksumFailed:
			// 校验失败, 需要重新下载
			result.NeedRetry = true
			// 设置允许覆盖
			dtu.IsOverwrite = true
			return
		default:
			result.NeedRetry = false
			return
		}
	}

	fmt.Printf("[%s] 检验文件有效性成功: %s\n", dtu.taskInfo.Id(), dtu.SavePath)
	return true
}

func (dtu *DownloadTaskUnit) OnRetry(lastRunResult *taskframework.TaskUnitRunResult) {
	// 输出错误信息
	if lastRunResult.Err == nil {
		// result中不包含Err, 忽略输出
		fmt.Printf("[%s] %s, 重试 %d/%d\n", dtu.taskInfo.Id(), lastRunResult.ResultMessage, dtu.taskInfo.Retry(), dtu.taskInfo.MaxRetry())
		return
	}
	fmt.Printf("[%s] %s, %s, 重试 %d/%d\n", dtu.taskInfo.Id(), lastRunResult.ResultMessage, lastRunResult.Err, dtu.taskInfo.Retry(), dtu.taskInfo.MaxRetry())
}

func (dtu *DownloadTaskUnit) OnSuccess(lastRunResult *taskframework.TaskUnitRunResult) {
}

func (dtu *DownloadTaskUnit) OnFailed(lastRunResult *taskframework.TaskUnitRunResult) {
	// 失败
	if lastRunResult.Err == nil {
		// result中不包含Err, 忽略输出
		fmt.Printf("[%s] %s\n", dtu.taskInfo.Id(), lastRunResult.ResultMessage)
		return
	}
	fmt.Printf("[%s] %s, %s\n", dtu.taskInfo.Id(), lastRunResult.ResultMessage, lastRunResult.Err)
}

func (dtu *DownloadTaskUnit) OnComplete(lastRunResult *taskframework.TaskUnitRunResult) {
}

func (dtu *DownloadTaskUnit) RetryWait() time.Duration {
	return functions.RetryWait(dtu.taskInfo.Retry())
}

func (dtu *DownloadTaskUnit) Run() (result *taskframework.TaskUnitRunResult) {
	result = &taskframework.TaskUnitRunResult{}
	// 获取文件信息
	var apierr *apierror.ApiError
	if dtu.fileInfo == nil || dtu.taskInfo.Retry() > 0 {
		// 没有获取文件信息
		// 如果是动态添加的下载任务, 是会写入文件信息的
		// 如果该任务重试过, 则应该再获取一次文件信息
		dtu.fileInfo, apierr = dtu.PanClient.AppFileInfoByPath(dtu.FamilyId, dtu.FilePanPath)
		if apierr != nil {
			// 如果不是未登录或文件不存在, 则不重试
			result.ResultMessage = "获取下载路径信息错误"
			result.Err = apierr
			dtu.handleError(result)
			return
		}
	}

	// 输出文件信息
	fmt.Print("\n")
	fmt.Printf("[%s] ----\n%s\n", dtu.taskInfo.Id(), dtu.fileInfo.String())

	// 如果是一个目录, 将子文件和子目录加入队列
	if dtu.fileInfo.IsFolder {
		_, err := os.Stat(dtu.SavePath)
		if err != nil && !os.IsExist(err) {
			os.MkdirAll(dtu.SavePath, 0777) // 首先在本地创建目录, 保证空目录也能被保存
		}

		// 获取该目录下的文件列表
		fileList := dtu.PanClient.AppFilesDirectoriesRecurseList(dtu.FamilyId, dtu.FilePanPath, nil)
		if fileList == nil {
			result.ResultMessage = "获取目录信息错误"
			result.Err = err
			result.NeedRetry = true
			return
		}

		for k := range fileList {
			if fileList[k].IsFolder {
				continue
			}
			// 添加子任务
			subUnit := *dtu
			newCfg := *dtu.Cfg
			subUnit.Cfg = &newCfg
			subUnit.fileInfo = fileList[k] // 保存文件信息
			subUnit.FilePanPath = fileList[k].Path
			subUnit.SavePath = filepath.Join(dtu.OriginSaveRootPath, fileList[k].Path) // 保存位置

			// 加入父队列
			info := dtu.ParentTaskExecutor.Append(&subUnit, dtu.taskInfo.MaxRetry())
			fmt.Printf("[%s] 加入下载队列: %s\n", info.Id(), fileList[k].Path)
		}

		result.Succeed = true // 执行成功
		return
	}

	fmt.Printf("[%s] 准备下载: %s\n", dtu.taskInfo.Id(), dtu.FilePanPath)

	if !dtu.IsOverwrite && FileExist(dtu.SavePath) {
		fmt.Printf("[%s] 文件已经存在: %s, 跳过...\n", dtu.taskInfo.Id(), dtu.SavePath)
		result.Succeed = true // 执行成功
		return
	}

	fmt.Printf("[%s] 将会下载到路径: %s\n\n", dtu.taskInfo.Id(), dtu.SavePath)

	var ok bool
	er := dtu.download()

	if er != nil {
		// 以上执行不成功, 返回
		result.ResultMessage = StrDownloadFailed
		result.Err = er
		dtu.handleError(result)
		return result
	}

	// 检测文件有效性
	ok = dtu.checkFileValid(result)
	if !ok {
		// 校验不成功, 返回结果
		return result
	}

	// 统计下载
	dtu.DownloadStatistic.AddTotalSize(dtu.fileInfo.FileSize)
	// 下载成功
	result.Succeed = true
	return
}
