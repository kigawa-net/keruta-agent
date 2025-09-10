package net.kigawa.keruta.utils

import java.io.File
import java.nio.file.Files
import java.nio.file.Path
import kotlin.io.path.exists
import kotlin.io.path.isDirectory
import kotlin.io.path.isRegularFile

object FileUtils {
    
    fun fileExists(path: String): Boolean {
        return File(path).exists()
    }
    
    fun fileExists(path: Path): Boolean {
        return path.exists() && path.isRegularFile()
    }
    
    fun dirExists(path: String): Boolean {
        return File(path).isDirectory
    }
    
    fun dirExists(path: Path): Boolean {
        return path.exists() && path.isDirectory()
    }
    
    fun createDirectories(path: String) {
        File(path).mkdirs()
    }
    
    fun createDirectories(path: Path) {
        Files.createDirectories(path)
    }
    
    fun deleteFile(path: String): Boolean {
        return File(path).delete()
    }
    
    fun deleteFile(path: Path): Boolean {
        return try {
            Files.delete(path)
            true
        } catch (e: Exception) {
            false
        }
    }
    
    fun copyFile(source: String, target: String) {
        File(source).copyTo(File(target), overwrite = true)
    }
    
    fun copyFile(source: Path, target: Path) {
        Files.copy(source, target)
    }
    
    fun moveFile(source: String, target: String) {
        File(source).renameTo(File(target))
    }
    
    fun moveFile(source: Path, target: Path) {
        Files.move(source, target)
    }
    
    fun getFileSize(path: String): Long {
        return File(path).length()
    }
    
    fun getFileSize(path: Path): Long {
        return Files.size(path)
    }
    
    fun readFileAsString(path: String): String {
        return File(path).readText()
    }
    
    fun readFileAsString(path: Path): String {
        return Files.readString(path)
    }
    
    fun writeStringToFile(path: String, content: String) {
        File(path).writeText(content)
    }
    
    fun writeStringToFile(path: Path, content: String) {
        Files.writeString(path, content)
    }
}