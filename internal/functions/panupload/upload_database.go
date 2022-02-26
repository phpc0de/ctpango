// Copyright (c) 2020 tickstep & chenall
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
package panupload

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phpc0de/ctpango/internal/config"
	"github.com/phpc0de/ctpango/internal/file/uploader"
	"github.com/phpc0de/ctpango/internal/localfile"
	"github.com/phpc0de/ctlibgo/converter"
	"github.com/phpc0de/ctlibgo/jsonhelper"
)

type (
	// Uploading 未完成上传的信息
	Uploading struct {
		*localfile.LocalFileMeta
		State *uploader.InstanceState `json:"state"`
	}

	// UploadingDatabase 未完成上传的数据库
	UploadingDatabase struct {
		UploadingList []*Uploading `json:"upload_state"`
		Timestamp     int64        `json:"timestamp"`

		dataFile *os.File
	}
)

// NewUploadingDatabase 初始化未完成上传的数据库, 从库中读取内容
func NewUploadingDatabase() (ud *UploadingDatabase, err error) {
	file, err := os.OpenFile(filepath.Join(config.GetConfigDir(), UploadingFileName), os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		return nil, err
	}

	ud = &UploadingDatabase{
		dataFile: file,
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if info.Size() <= 0 {
		return ud, nil
	}

	err = jsonhelper.UnmarshalData(file, ud)
	if err != nil {
		return nil, err
	}

	return ud, nil
}

// Save 保存内容
func (ud *UploadingDatabase) Save() error {
	if ud.dataFile == nil {
		return errors.New("dataFile is nil")
	}

	ud.Timestamp = time.Now().Unix()

	var (
		builder = &strings.Builder{}
		err     = jsonhelper.MarshalData(builder, ud)
	)
	if err != nil {
		panic(err)
	}

	err = ud.dataFile.Truncate(int64(builder.Len()))
	if err != nil {
		return err
	}

	str := builder.String()
	_, err = ud.dataFile.WriteAt(converter.ToBytes(str), 0)
	if err != nil {
		return err
	}

	return nil
}

// UpdateUploading 更新正在上传
func (ud *UploadingDatabase) UpdateUploading(meta *localfile.LocalFileMeta, state *uploader.InstanceState) {
	if meta == nil {
		return
	}

	meta.CompleteAbsPath()
	for k, uploading := range ud.UploadingList {
		if uploading.LocalFileMeta == nil {
			continue
		}
		if uploading.LocalFileMeta.EqualLengthMD5(meta) || uploading.LocalFileMeta.Path == meta.Path {
			ud.UploadingList[k].State = state
			return
		}
	}

	ud.UploadingList = append(ud.UploadingList, &Uploading{
		LocalFileMeta: meta,
		State:         state,
	})
}

func (ud *UploadingDatabase) deleteIndex(k int) {
	ud.UploadingList = append(ud.UploadingList[:k], ud.UploadingList[k+1:]...)
}

// Delete 删除
func (ud *UploadingDatabase) Delete(meta *localfile.LocalFileMeta) bool {
	if meta == nil {
		return false
	}

	meta.CompleteAbsPath()
	for k, uploading := range ud.UploadingList {
		if uploading.LocalFileMeta == nil {
			continue
		}
		if uploading.LocalFileMeta.EqualLengthMD5(meta) || uploading.LocalFileMeta.Path == meta.Path {
			ud.deleteIndex(k)
			return true
		}
	}
	return false
}

// Search 搜索
func (ud *UploadingDatabase) Search(meta *localfile.LocalFileMeta) *uploader.InstanceState {
	if meta == nil {
		return nil
	}

	meta.CompleteAbsPath()
	ud.clearModTimeChange()
	for _, uploading := range ud.UploadingList {
		if uploading.LocalFileMeta == nil {
			continue
		}
		if uploading.LocalFileMeta.EqualLengthMD5(meta) {
			return uploading.State
		}
		if uploading.LocalFileMeta.Path == meta.Path {
			// 移除旧的信息
			// 目前只是比较了文件大小
			if meta.Length != uploading.LocalFileMeta.Length {
				ud.Delete(meta)
				return nil
			}

			// 覆盖数据
			meta.MD5 = uploading.LocalFileMeta.MD5
			meta.ParentFolderId = uploading.LocalFileMeta.ParentFolderId
			meta.UploadFileId = uploading.LocalFileMeta.UploadFileId
			meta.FileUploadUrl = uploading.LocalFileMeta.FileUploadUrl
			meta.FileCommitUrl = uploading.LocalFileMeta.FileCommitUrl
			meta.FileDataExists = uploading.LocalFileMeta.FileDataExists
			meta.XRequestId = uploading.LocalFileMeta.XRequestId
			return uploading.State
		}
	}
	return nil
}

func (ud *UploadingDatabase) clearModTimeChange() {
	for i := 0; i < len(ud.UploadingList); i++ {
		uploading := ud.UploadingList[i]
		if uploading.LocalFileMeta == nil {
			continue
		}

		if uploading.ModTime == -1 { // 忽略
			continue
		}

		info, err := os.Stat(uploading.LocalFileMeta.Path)
		if err != nil {
			ud.deleteIndex(i)
			i--
			cmdUploadVerbose.Warnf("clear invalid file path: %s, err: %s\n", uploading.LocalFileMeta.Path, err)
			continue
		}

		if uploading.LocalFileMeta.ModTime != info.ModTime().Unix() {
			ud.deleteIndex(i)
			i--
			cmdUploadVerbose.Infof("clear modified file path: %s\n", uploading.LocalFileMeta.Path)
			continue
		}
	}
}

// Close 关闭数据库
func (ud *UploadingDatabase) Close() error {
	return ud.dataFile.Close()
}
