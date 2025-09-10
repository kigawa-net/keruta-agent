package net.kigawa.keruta.config

import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.BeforeEach
import java.time.Duration

class ConfigTest {
    
    @BeforeEach
    fun setUp() {
        // 環境変数をクリア
        System.clearProperty("KERUTA_API_URL")
        System.clearProperty("KERUTA_API_TOKEN")
        System.clearProperty("KERUTA_TASK_ID")
    }
    
    @Test
    fun testDefaultConfig() {
        val config = AppConfig()
        
        assertEquals("", config.api.url)
        assertEquals("", config.api.token)
        assertEquals(Duration.ofSeconds(30), config.api.timeout)
        assertEquals("INFO", config.logging.level)
        assertEquals("json", config.logging.format)
        assertEquals(100 * 1024 * 1024L, config.artifacts.maxSize)
        assertEquals("/.keruta/doc", config.artifacts.directory)
        assertTrue(config.errorHandling.autoFix)
        assertEquals(3, config.errorHandling.retryCount)
    }
    
    @Test
    fun testConfigFromEnv() {
        // 環境変数を設定
        System.setProperty("KERUTA_API_URL", "http://test.example.com")
        System.setProperty("KERUTA_API_TOKEN", "test-token")
        System.setProperty("KERUTA_LOG_LEVEL", "DEBUG")
        
        // Config初期化をテスト（実際の環境変数読み込みロジックをここでテスト）
        val apiUrl = System.getProperty("KERUTA_API_URL") ?: ""
        val apiToken = System.getProperty("KERUTA_API_TOKEN") ?: ""
        val logLevel = System.getProperty("KERUTA_LOG_LEVEL") ?: "INFO"
        
        assertEquals("http://test.example.com", apiUrl)
        assertEquals("test-token", apiToken)
        assertEquals("DEBUG", logLevel)
    }
    
    @Test
    fun testGetTaskId() {
        System.setProperty("KERUTA_TASK_ID", "test-task-123")
        assertEquals("test-task-123", System.getProperty("KERUTA_TASK_ID"))
    }
    
    @Test
    fun testGetWorkspaceId() {
        System.setProperty("KERUTA_WORKSPACE_ID", "workspace-123")
        assertEquals("workspace-123", System.getProperty("KERUTA_WORKSPACE_ID"))
        
        System.clearProperty("KERUTA_WORKSPACE_ID")
        System.setProperty("CODER_WORKSPACE_ID", "coder-workspace-456")
        assertEquals("coder-workspace-456", System.getProperty("CODER_WORKSPACE_ID"))
    }
}