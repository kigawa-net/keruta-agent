package net.kigawa.keruta.utils

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import org.junit.jupiter.api.AfterEach
import org.junit.jupiter.api.io.TempDir
import java.io.File
import java.nio.file.Path
import kotlin.io.path.createFile
import kotlin.io.path.writeText

class FileUtilsTest {
    
    @TempDir
    lateinit var tempDir: Path
    
    @Test
    fun testFileExists() {
        // 存在するファイルのテスト
        val existingFile = tempDir.resolve("existing-file.txt")
        existingFile.createFile()
        
        assertTrue(FileUtils.fileExists(existingFile.toString()))
        assertTrue(FileUtils.fileExists(existingFile))
        
        // 存在しないファイルのテスト
        val nonExistingFile = tempDir.resolve("non-existing-file.txt")
        assertFalse(FileUtils.fileExists(nonExistingFile.toString()))
        assertFalse(FileUtils.fileExists(nonExistingFile))
    }
    
    @Test
    fun testDirExists() {
        // 存在するディレクトリのテスト
        assertTrue(FileUtils.dirExists(tempDir.toString()))
        assertTrue(FileUtils.dirExists(tempDir))
        
        // 存在しないディレクトリのテスト
        val nonExistingDir = tempDir.resolve("non-existing-dir")
        assertFalse(FileUtils.dirExists(nonExistingDir.toString()))
        assertFalse(FileUtils.dirExists(nonExistingDir))
    }
    
    @Test
    fun testCreateDirectories() {
        val newDir = tempDir.resolve("new-directory")
        assertFalse(FileUtils.dirExists(newDir))
        
        FileUtils.createDirectories(newDir.toString())
        assertTrue(FileUtils.dirExists(newDir))
        
        val anotherDir = tempDir.resolve("another-directory")
        FileUtils.createDirectories(anotherDir)
        assertTrue(FileUtils.dirExists(anotherDir))
    }
    
    @Test
    fun testReadWriteFile() {
        val testFile = tempDir.resolve("test-content.txt")
        val content = "Hello, Kotlin World!"
        
        // 文字列でのファイル操作テスト
        FileUtils.writeStringToFile(testFile.toString(), content)
        assertTrue(FileUtils.fileExists(testFile.toString()))
        
        val readContent = FileUtils.readFileAsString(testFile.toString())
        assertEquals(content, readContent)
        
        // Pathでのファイル操作テスト
        val anotherFile = tempDir.resolve("another-test.txt")
        FileUtils.writeStringToFile(anotherFile, content)
        assertTrue(FileUtils.fileExists(anotherFile))
        
        val anotherReadContent = FileUtils.readFileAsString(anotherFile)
        assertEquals(content, anotherReadContent)
    }
    
    @Test
    fun testGetFileSize() {
        val testFile = tempDir.resolve("size-test.txt")
        val content = "Test content for size calculation"
        
        testFile.writeText(content)
        
        val expectedSize = content.toByteArray().size.toLong()
        assertEquals(expectedSize, FileUtils.getFileSize(testFile.toString()))
        assertEquals(expectedSize, FileUtils.getFileSize(testFile))
    }
    
    @Test
    fun testCopyFile() {
        val sourceFile = tempDir.resolve("source.txt")
        val targetFile = tempDir.resolve("target.txt")
        val content = "Content to be copied"
        
        sourceFile.writeText(content)
        
        // 文字列でのコピーテスト
        FileUtils.copyFile(sourceFile.toString(), targetFile.toString())
        assertTrue(FileUtils.fileExists(targetFile.toString()))
        assertEquals(content, FileUtils.readFileAsString(targetFile.toString()))
        
        // Pathでのコピーテスト
        val anotherTargetFile = tempDir.resolve("another-target.txt")
        FileUtils.copyFile(sourceFile, anotherTargetFile)
        assertTrue(FileUtils.fileExists(anotherTargetFile))
        assertEquals(content, FileUtils.readFileAsString(anotherTargetFile))
    }
    
    @Test
    fun testDeleteFile() {
        val testFile = tempDir.resolve("to-be-deleted.txt")
        testFile.createFile()
        
        assertTrue(FileUtils.fileExists(testFile))
        assertTrue(FileUtils.deleteFile(testFile.toString()))
        assertFalse(FileUtils.fileExists(testFile))
        
        // 存在しないファイルの削除テスト
        assertFalse(FileUtils.deleteFile(testFile.toString()))
    }
}