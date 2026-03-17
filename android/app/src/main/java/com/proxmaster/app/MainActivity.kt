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
            text = "State: All"
            setOnClickListener {
                runInBackground {
                    api.get("/state/all")
                }
            }
        }

        val btnSpecWorkload = Button(this).apply {
            text = "Spec: Workload pfSense"
            setOnClickListener {
                runInBackground {
                    api.put(
                        "/spec/workloads/pfsense-gw",
                        "{\"id\":\"pfsense-gw\",\"name\":\"pfsense-gw\",\"kind\":\"vm\",\"node_id\":\"node-1\",\"cpu\":2,\"memory_mb\":4096,\"disk_gb\":20,\"desired_power\":\"running\"}"
                    )
                }
            }
        }

        val wizardTitle = TextView(this).apply { text = "Wizard Mode" }

        val btnWizardBlueprintPlan = Button(this).apply {
            text = "Wizard: Plan pfSense Blueprint"
            setOnClickListener {
                runInBackground {
                    api.post(
                        "/blueprints/plan",
                        "{\"name\":\"pfsense-gateway\",\"node_id\":\"node-1\",\"workload_name\":\"pfsense-gw\"}"
                    )
                }
            }
        }

        val btnWizardBlueprintDeploy = Button(this).apply {
            text = "Wizard: Deploy pfSense Blueprint"
            setOnClickListener {
                runInBackground {
                    api.post(
                        "/blueprints/deploy",
                        "{\"name\":\"pfsense-gateway\",\"node_id\":\"node-1\",\"workload_name\":\"pfsense-gw\"}"
                    )
                }
            }
        }

        val expertTitle = TextView(this).apply { text = "Expert Mode" }

        val btnExpertCatalog = Button(this).apply {
            text = "Expert: Blueprint Catalog"
            setOnClickListener {
                runInBackground {
                    api.get("/blueprints")
                }
            }
        }

        val btnExpertAggressive = Button(this).apply {
            text = "Expert: Aggressive Auto 30m"
            setOnClickListener {
                runInBackground {
                    api.post(
                        "/policy/mode",
                        "{\"mode\":\"AGGRESSIVE_AUTO\",\"duration_minutes\":30,\"reauth_token\":\"reauth-ok\",\"hardware_mfa\":true,\"second_approver\":\"ops-admin\"}"
                    )
                }
            }
        }

        val btnExpertTimeline = Button(this).apply {
            text = "Expert: Jobs Timeline"
            setOnClickListener {
                runInBackground {
                    api.get("/jobs/timeline")
                }
            }
        }

        output = TextView(this).apply {
            text = "Proxmaster ready"
        }

        root.addView(btnState)
        root.addView(btnSpecWorkload)
        root.addView(wizardTitle)
        root.addView(btnWizardBlueprintPlan)
        root.addView(btnWizardBlueprintDeploy)
        root.addView(expertTitle)
        root.addView(btnExpertCatalog)
        root.addView(btnExpertAggressive)
        root.addView(btnExpertTimeline)
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
