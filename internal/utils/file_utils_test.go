package utils

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileExists(t *testing.T) {
	// テスト用の一時ファイルを作成
	tempFile, err := os.CreateTemp("", "test-file-exists-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// ファイルが存在する場合
	exists := FileExists(tempFile.Name())
	assert.True(t, exists)

	// ファイルが存在しない場合
	nonExistentFile := "/tmp/non-existent-file-12345.txt"
	exists = FileExists(nonExistentFile)
	assert.False(t, exists)
}

func TestDirExists(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-dir-exists-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// ディレクトリが存在する場合
	exists := DirExists(tempDir)
	assert.True(t, exists)

	// ディレクトリが存在しない場合
	nonExistentDir := "/tmp/non-existent-dir-12345"
	exists = DirExists(nonExistentDir)
	assert.False(t, exists)
}

func TestCreateDirIfNotExists(t *testing.T) {
	// テスト用のベースディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-create-dir-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 存在しないディレクトリを作成
	newDir := filepath.Join(tempDir, "new-directory")
	err = CreateDirIfNotExists(newDir)
	assert.NoError(t, err)

	// ディレクトリが作成されていることを確認
	assert.True(t, DirExists(newDir))

	// 既に存在するディレクトリで実行（エラーが発生しないことを確認）
	err = CreateDirIfNotExists(newDir)
	assert.NoError(t, err)
}

func TestCreateDirIfNotExistsNested(t *testing.T) {
	// テスト用のベースディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-create-nested-dir-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// ネストしたディレクトリを作成
	nestedDir := filepath.Join(tempDir, "level1", "level2", "level3")
	err = CreateDirIfNotExists(nestedDir)
	assert.NoError(t, err)

	// ディレクトリが作成されていることを確認
	assert.True(t, DirExists(nestedDir))
}

func TestCreateDirIfNotExistsPermissionError(t *testing.T) {
	// rootユーザーの場合はスキップ
	if os.Getuid() == 0 {
		t.Skip("Running as root, cannot test permission errors")
	}

	// 書き込み権限のないディレクトリでテスト
	restrictedDir := "/root/restricted-directory-test"
	err := CreateDirIfNotExists(restrictedDir)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "permission denied") || strings.Contains(err.Error(), "read-only file system"))
}

func TestGetFileSize(t *testing.T) {
	// テスト用の一時ファイルを作成
	tempFile, err := os.CreateTemp("", "test-file-size-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())

	// テストデータを書き込み
	testData := "Hello, World! This is a test file for size calculation."
	_, err = tempFile.WriteString(testData)
	require.NoError(t, err)
	tempFile.Close()

	// ファイルサイズを取得
	size, err := GetFileSize(tempFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, int64(len(testData)), size)
}

func TestGetFileSizeNonExistent(t *testing.T) {
	// 存在しないファイルでテスト
	nonExistentFile := "/tmp/non-existent-file-size-test-12345.txt"
	_, err := GetFileSize(nonExistentFile)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestCopyFile(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-copy-file-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// ソースファイルを作成
	srcFile := filepath.Join(tempDir, "source.txt")
	testData := "This is test data for file copying."
	err = os.WriteFile(srcFile, []byte(testData), 0644)
	require.NoError(t, err)

	// ファイルをコピー
	dstFile := filepath.Join(tempDir, "destination.txt")
	err = CopyFile(srcFile, dstFile)
	assert.NoError(t, err)

	// コピー先ファイルが存在することを確認
	assert.True(t, FileExists(dstFile))

	// コピー先ファイルの内容を確認
	copiedData, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, testData, string(copiedData))
}

func TestCopyFileSourceNotExists(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-copy-file-error-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// 存在しないソースファイルでテスト
	srcFile := filepath.Join(tempDir, "non-existent-source.txt")
	dstFile := filepath.Join(tempDir, "destination.txt")

	err = CopyFile(srcFile, dstFile)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestCopyFileDestinationDirNotExists(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-copy-file-dest-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// ソースファイルを作成
	srcFile := filepath.Join(tempDir, "source.txt")
	testData := "Test data for destination directory creation."
	err = os.WriteFile(srcFile, []byte(testData), 0644)
	require.NoError(t, err)

	// 存在しないディレクトリにファイルをコピー
	dstFile := filepath.Join(tempDir, "new-directory", "destination.txt")
	err = CopyFile(srcFile, dstFile)
	assert.NoError(t, err)

	// コピー先ファイルが存在することを確認
	assert.True(t, FileExists(dstFile))

	// コピー先ファイルの内容を確認
	copiedData, err := os.ReadFile(dstFile)
	require.NoError(t, err)
	assert.Equal(t, testData, string(copiedData))
}

func TestRemoveFile(t *testing.T) {
	// テスト用の一時ファイルを作成
	tempFile, err := os.CreateTemp("", "test-remove-file-*.txt")
	require.NoError(t, err)
	tempFileName := tempFile.Name()
	tempFile.Close()

	// ファイルが存在することを確認
	assert.True(t, FileExists(tempFileName))

	// ファイルを削除
	err = RemoveFile(tempFileName)
	assert.NoError(t, err)

	// ファイルが削除されていることを確認
	assert.False(t, FileExists(tempFileName))
}

func TestRemoveFileNonExistent(t *testing.T) {
	// 存在しないファイルを削除（エラーが発生することを期待）
	nonExistentFile := "/tmp/non-existent-remove-test-12345.txt"
	err := RemoveFile(nonExistentFile)
	// os.Remove は存在しないファイルに対してエラーを返す
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestRemoveDirectory(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-remove-dir-")
	require.NoError(t, err)

	// ディレクトリ内にファイルを作成
	testFile := filepath.Join(tempDir, "test-file.txt")
	err = os.WriteFile(testFile, []byte("test data"), 0644)
	require.NoError(t, err)

	// サブディレクトリを作成
	subDir := filepath.Join(tempDir, "sub-directory")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// サブディレクトリ内にファイルを作成
	subFile := filepath.Join(subDir, "sub-file.txt")
	err = os.WriteFile(subFile, []byte("sub data"), 0644)
	require.NoError(t, err)

	// ディレクトリが存在することを確認
	assert.True(t, DirExists(tempDir))

	// ディレクトリを削除
	err = RemoveDirectory(tempDir)
	assert.NoError(t, err)

	// ディレクトリが削除されていることを確認
	assert.False(t, DirExists(tempDir))
}

func TestRemoveDirectoryNonExistent(t *testing.T) {
	// 存在しないディレクトリを削除
	nonExistentDir := "/tmp/non-existent-remove-dir-test-12345"
	err := RemoveDirectory(nonExistentDir)
	assert.NoError(t, err) // os.RemoveAll は存在しないディレクトリでもエラーにならない
}

func TestGetFilePermissions(t *testing.T) {
	// テスト用の一時ファイルを作成
	tempFile, err := os.CreateTemp("", "test-permissions-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// ファイルの権限を設定
	err = os.Chmod(tempFile.Name(), 0644)
	require.NoError(t, err)

	// ファイルの権限を取得
	perms, err := GetFilePermissions(tempFile.Name())
	assert.NoError(t, err)
	assert.Equal(t, fs.FileMode(0644), perms)
}

func TestGetFilePermissionsNonExistent(t *testing.T) {
	// 存在しないファイルでテスト
	nonExistentFile := "/tmp/non-existent-permissions-test-12345.txt"
	_, err := GetFilePermissions(nonExistentFile)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestChangeFilePermissions(t *testing.T) {
	// テスト用の一時ファイルを作成
	tempFile, err := os.CreateTemp("", "test-change-permissions-*.txt")
	require.NoError(t, err)
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// 初期権限を設定
	err = os.Chmod(tempFile.Name(), 0644)
	require.NoError(t, err)

	// 権限を変更
	newPerms := fs.FileMode(0755)
	err = ChangeFilePermissions(tempFile.Name(), newPerms)
	assert.NoError(t, err)

	// 変更された権限を確認
	actualPerms, err := GetFilePermissions(tempFile.Name())
	require.NoError(t, err)
	assert.Equal(t, newPerms, actualPerms)
}

func TestChangeFilePermissionsNonExistent(t *testing.T) {
	// 存在しないファイルでテスト
	nonExistentFile := "/tmp/non-existent-change-permissions-test-12345.txt"
	err := ChangeFilePermissions(nonExistentFile, 0755)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestListFiles(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-list-files-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// テストファイルを作成
	testFiles := []string{"file1.txt", "file2.log", "file3.json"}
	for _, fileName := range testFiles {
		filePath := filepath.Join(tempDir, fileName)
		err = os.WriteFile(filePath, []byte("test content"), 0644)
		require.NoError(t, err)
	}

	// サブディレクトリを作成
	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	// ファイル一覧を取得
	files, err := ListFiles(tempDir)
	assert.NoError(t, err)
	assert.Len(t, files, 4) // 3つのファイル + 1つのディレクトリ

	// ファイル名が含まれていることを確認
	fileNames := make([]string, len(files))
	for i, file := range files {
		fileNames[i] = file.Name()
	}

	for _, expectedFile := range testFiles {
		assert.Contains(t, fileNames, expectedFile)
	}
	assert.Contains(t, fileNames, "subdir")
}

func TestListFilesNonExistent(t *testing.T) {
	// 存在しないディレクトリでテスト
	nonExistentDir := "/tmp/non-existent-list-files-test-12345"
	_, err := ListFiles(nonExistentDir)
	assert.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestListFilesRecursive(t *testing.T) {
	// テスト用の一時ディレクトリを作成
	tempDir, err := os.MkdirTemp("", "test-list-files-recursive-")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// ルートレベルのファイルを作成
	rootFile := filepath.Join(tempDir, "root.txt")
	err = os.WriteFile(rootFile, []byte("root content"), 0644)
	require.NoError(t, err)

	// サブディレクトリとファイルを作成
	subDir := filepath.Join(tempDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	subFile := filepath.Join(subDir, "sub.txt")
	err = os.WriteFile(subFile, []byte("sub content"), 0644)
	require.NoError(t, err)

	// ネストしたサブディレクトリとファイルを作成
	nestedDir := filepath.Join(subDir, "nested")
	err = os.Mkdir(nestedDir, 0755)
	require.NoError(t, err)

	nestedFile := filepath.Join(nestedDir, "nested.txt")
	err = os.WriteFile(nestedFile, []byte("nested content"), 0644)
	require.NoError(t, err)

	// 再帰的にファイル一覧を取得
	allFiles, err := ListFilesRecursive(tempDir)
	assert.NoError(t, err)
	assert.Len(t, allFiles, 6) // root.txt, subdir/, sub.txt, nested/, nested.txt, + tempDir自体

	// 期待されるファイルが含まれていることを確認
	filePaths := make([]string, len(allFiles))
	for i, file := range allFiles {
		filePaths[i] = file.Path
	}

	assert.Contains(t, filePaths, rootFile)
	assert.Contains(t, filePaths, subFile)
	assert.Contains(t, filePaths, nestedFile)
}

// ヘルパー関数の実装（実際のutilsパッケージにも実装が必要）

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func CreateDirIfNotExists(path string) error {
	if !DirExists(path) {
		return os.MkdirAll(path, 0755)
	}
	return nil
}

func GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func CopyFile(src, dst string) error {
	// 宛先ディレクトリが存在しない場合は作成
	dstDir := filepath.Dir(dst)
	if err := CreateDirIfNotExists(dstDir); err != nil {
		return err
	}

	// ソースファイルを読み込み
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	// 宛先ファイルに書き込み
	return os.WriteFile(dst, data, 0644)
}

func RemoveFile(path string) error {
	return os.Remove(path)
}

func RemoveDirectory(path string) error {
	return os.RemoveAll(path)
}

func GetFilePermissions(path string) (fs.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Mode(), nil
}

func ChangeFilePermissions(path string, mode fs.FileMode) error {
	return os.Chmod(path, mode)
}

func ListFiles(dir string) ([]fs.DirEntry, error) {
	return os.ReadDir(dir)
}

type FileInfo struct {
	Path string
	Info fs.FileInfo
}

func ListFilesRecursive(root string) ([]FileInfo, error) {
	var files []FileInfo
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files = append(files, FileInfo{Path: path, Info: info})
		return nil
	})
	return files, err
}
