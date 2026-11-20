"""Offline checks for Konflux integration-test scenario triggers."""

import unittest
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parents[2]
SCENARIOS = (
    Path(__file__).parent
    / "integration-test-scenarios/konflux-integration-test-scenarios.yaml"
)


class ScenarioTests(unittest.TestCase):
    def test_e2e_scenarios_only_run_for_pull_requests(self):
        scenarios = list(yaml.safe_load_all(SCENARIOS.read_text()))
        self.assertEqual(len(scenarios), 21)
        for scenario in scenarios:
            with self.subTest(name=scenario["metadata"]["name"]):
                self.assertEqual(
                    [context["name"] for context in scenario["spec"]["contexts"]],
                    ["pull_request"],
                )
                resolver = scenario["spec"]["resolverRef"]["params"]
                path = next(param["value"] for param in resolver if param["name"] == "pathInRepo")
                pipeline_path = ROOT / path
                self.assertTrue(pipeline_path.is_file(), path)
                pipeline = yaml.safe_load(pipeline_path.read_text())
                tasks = {task["name"]: task for task in pipeline["spec"]["tasks"]}
                gate = "check-snapshot-component"
                self.assertIn(gate, tasks)
                self.assertIn(
                    "metadata.labels['appstudio.openshift.io/component']",
                    str(tasks[gate]),
                )
                self.assertEqual(
                    tasks["provision-ephemeral-cluster"]["when"],
                    [{
                        "input": f"$(tasks.{gate}.results.matches)",
                        "operator": "in",
                        "values": ["true"],
                    }],
                )
                self.assertEqual(
                    pipeline["spec"]["finally"][-1]["when"],
                    tasks["provision-ephemeral-cluster"]["when"],
                )


if __name__ == "__main__":
    unittest.main()
