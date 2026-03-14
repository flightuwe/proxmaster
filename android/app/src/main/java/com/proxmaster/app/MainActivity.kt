package com.proxmaster.app

import android.os.Bundle
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity

class MainActivity : AppCompatActivity() {
    private lateinit var output: TextView
    private val api = ProxmasterApi()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(32, 32, 32, 32)
        }

        val btnState = Button(this).apply {
            text = "Cluster State"
            setOnClickListener {
                runInBackground {
                    api.callTool(
                        tool = "cluster.get_state",
                        params = "{}"
                    )
                }
            }
        }

        val btnMigrate = Button(this).apply {
            text = "Migrate VM 101 -> node-2"
            setOnClickListener {
                runInBackground {
                    api.callTool(
                        tool = "vm.migrate",
                        params = "{\"vm_id\":\"101\",\"target_node\":\"node-2\"}"
                    )
                }
            }
        }

        output = TextView(this).apply {
            text = "Proxmaster ready"
        }

        root.addView(btnState)
        root.addView(btnMigrate)
        root.addView(output)

        val scroll = ScrollView(this).apply { addView(root) }
        setContentView(scroll)
    }

    private fun runInBackground(block: () -> String) {
        Thread {
            val result = try {
                block()
            } catch (e: Exception) {
                "Error: ${e.message}"
            }
            runOnUiThread {
                output.text = result
            }
        }.start()
    }
}