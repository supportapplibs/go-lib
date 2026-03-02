package storage

// func TestMinioUploadError(t *testing.T) {
// 	ctx := log.Off()
// 	minioClient, err := minio.New("localhost:9000", &minio.Options{
// 		Creds: credentials.NewStaticV4(
// 			"minioadmin",
// 			"minioadmin",
// 			"",
// 		),
// 		Secure: false,
// 	})
// 	assert.NoError(t, err)
// 	file := &filesystem.File{}
// 	assert.NoError(t, err)
// 	err = storage.MinioUpload(ctx, minioClient, file, "2025/12/30")
// 	assert.Error(t, err)
// }
