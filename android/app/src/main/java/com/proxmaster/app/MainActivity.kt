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

        val wizardTitle = TextView(this).apply { text = "Wizard Mode" }

        val btnWizardRebuildPlan = Button(this).apply {
            text = "Wizard: Plan Rebuild All Pools"
            setOnClickListener {
                runInBackground {
                    api.callApproveTool(
                        tool = "storage.pool.rebuild_all.plan",
                        params = "{}"
                    )
                }
            }
        }

        val btnWizardPolicy = Button(this).apply {
            text = "Wizard: Backup Policy VM 101"
            setOnClickListener {
                runInBackground {
                    api.callApproveTool(
                        tool = "backup.policy.upsert",
                        params = "{\"workload_id\":\"101\",\"workload_kind\":\"vm\",\"priority\":200,\"override\":true,\"schedule\":\"0 2 * * *\",\"target_id\":\"target-pbs-1\",\"rpo\":\"24h\",\"retention\":\"30d\",\"encryption\":true,\"immutability\":true,\"verify_restore\":true}"
                    )
                }
            }
        }

        val expertTitle = TextView(this).apply { text = "Expert Mode" }

        val btnExpertInventory = Button(this).apply {
            text = "Expert: Storage Inventory Sync"
            setOnClickListener {
                runInBackground {
                    api.callTool(
                        tool = "storage.inventory.sync",
                        params = "{}"
                    )
                }
            }
        }

        val btnExpertRestorePlan = Button(this).apply {
            text = "Expert: Plan Restore VM 101"
            setOnClickListener {
                runInBackground {
                    api.callApproveTool(
                        tool = "backup.restore.plan",
                        params = "{\"workload_id\":\"101\",\"target_id\":\"target-pbs-1\"}"
                    )
                }
            }
        }

        output = TextView(this).apply {
            text = "Proxmaster ready"
        }

        root.addView(btnState)
        root.addView(btnMigrate)
        root.addView(wizardTitle)
        root.addView(btnWizardRebuildPlan)
        root.addView(btnWizardPolicy)
        root.addView(expertTitle)
        root.addView(btnExpertInventory)
        root.addView(btnExpertRestorePlan)
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
