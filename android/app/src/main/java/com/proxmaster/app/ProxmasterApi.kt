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

    fun callApproveTool(tool: String, params: String): String {
        val body = """
            {
              "tool": "$tool",
              "params": $params,
              "actor": "android-admin",
              "reauth_token": "reauth-ok",
              "hardware_mfa": true,
              "second_approver": "ops-admin"
            }
        """.trimIndent().toRequestBody("application/json".toMediaType())

        val request = Request.Builder()
            .url("$baseUrl/mcp/approve")
            .addHeader("Authorization", "Bearer $token")
            .post(body)
            .build()

        client.newCall(request).execute().use { resp ->
            return resp.body?.string() ?: "no body"
        }
    }

    fun get(path: String): String {
        val request = Request.Builder()
            .url("$baseUrl$path")
            .addHeader("Authorization", "Bearer $token")
            .get()
            .build()
        client.newCall(request).execute().use { resp ->
            return resp.body?.string() ?: "no body"
        }
    }

    fun post(path: String, jsonBody: String): String {
        val request = Request.Builder()
            .url("$baseUrl$path")
            .addHeader("Authorization", "Bearer $token")
            .post(jsonBody.toRequestBody("application/json".toMediaType()))
            .build()
        client.newCall(request).execute().use { resp ->
            return resp.body?.string() ?: "no body"
        }
    }

    fun put(path: String, jsonBody: String): String {
        val request = Request.Builder()
            .url("$baseUrl$path")
            .addHeader("Authorization", "Bearer $token")
            .put(jsonBody.toRequestBody("application/json".toMediaType()))
            .build()
        client.newCall(request).execute().use { resp ->
            return resp.body?.string() ?: "no body"
        }
    }
}
