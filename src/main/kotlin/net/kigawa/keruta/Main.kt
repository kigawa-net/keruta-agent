package net.kigawa.keruta

import net.kigawa.keruta.commands.execute
import net.kigawa.keruta.config.Config
import net.kigawa.keruta.logger.Logger
import org.slf4j.LoggerFactory
import kotlin.system.exitProcess

private val logger = LoggerFactory.getLogger("Main")

fun main() {
    try {
        // 設定の初期化
        Config.init()

        // ロガーの初期化
        Logger.init()

        // ルートコマンドの実行
        execute()
    } catch (e: Exception) {
        logger.error("コマンドの実行に失敗しました", e)
        exitProcess(1)
    }
}
