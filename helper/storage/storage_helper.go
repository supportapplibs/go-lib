package storage

import (
	"context"
	"strings"

	"github.com/goravel/framework/contracts/filesystem"
	"github.com/minio/minio-go/v7"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type nfsHelper struct {
	err    error
	ctx    context.Context
	driver string
}
type minioHelper struct {
	err         error
	minioClient *minio.Client
	bucketName  string
	ctx         context.Context
}
type StorageHelper interface {
	Upload(file filesystem.File, dirpath, filename string) error
	Move(srcPath string, destPath string) error
	Copy(srcPath string, destPath string) error
	Delete(filepath string) error
	Securelink(filepath string) (string, error)
	SecurelinkFolder(dirpath string) (string, error)
}

func Disk(ctx context.Context, diskConfig string) StorageHelper {
	if strings.Contains(diskConfig, "minio") {
		return ConfigureMinio(ctx, diskConfig)
	}
	return &nfsHelper{ctx: ctx, driver: diskConfig}
}
