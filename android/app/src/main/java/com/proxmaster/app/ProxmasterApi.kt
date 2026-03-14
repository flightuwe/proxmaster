package com.proxmaster.app

import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

class ProxmasterApi {
    private val client = OkHttpClient()
    private val baseUrl = "http://100.100.100.10:8080"
    private val token = "dev-admin-token"

    fun callTool(tool: String, params: String): String {
        val body = """
            {
              "tool": "$tool",
              "params": $params,
              "actor": "android-admin"
            }
        """.trimIndent().toRequestBody("application/json".toMediaType())

        val request = Request.Builder()
            .url("$baseUrl/mcp/call")
            .addHeader("Authorization", "Bearer $token")
            .post(body)
            .build()

        client.newCall(request).execute().use { resp ->
            return resp.body?.string() ?: "no body"
        }
    }
}