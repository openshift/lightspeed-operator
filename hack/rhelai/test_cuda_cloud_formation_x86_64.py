"""Offline regression checks for the CUDA template and documented US AMIs."""

from pathlib import Path
import re
import unittest

DIRECTORY = Path(__file__).resolve().parent
EXPECTED_AMIS = {
    "us-east-1": "ami-0225644bfadc204f7",
    "us-east-2": "ami-0fa0cf9725e3938ae",
    "us-west-1": "ami-0fd6ac6c5f71fe86e",
    "us-west-2": "ami-02990c099b7180454",
}


class CudaTemplateTests(unittest.TestCase):
    """Check exact published IDs and ingress restrictions without AWS calls."""

    @classmethod
    def setUpClass(cls):
        cls.template = (DIRECTORY / "cuda_cloud_formation_x86_64.yaml").read_text()
        cls.readme = (DIRECTORY / "README.txt").read_text()
        cls.cidr_pattern = re.search(
            r"AllowedPattern: '([^']+)'",
            cls.template.split("  AllowedIngressCidr:", 1)[1],
        )[1]

    def test_us_ami_mappings(self):
        mapping = self.template.split("  RegionAMIs:\n", 1)[1].split(
            "\nResources:\n", 1
        )[0]
        for region, image in EXPECTED_AMIS.items():
            with self.subTest(region=region):
                match = re.search(
                    rf"^    {region}:\n      X8664: (ami-[0-9a-f]+)$", mapping, re.M
                )
                self.assertIsNotNone(match)
                self.assertEqual(match[1], image)
                self.assertIn(f"| {region} | {image} |", self.readme)

    def test_rejects_all_worldwide_ingress_spellings(self):
        for address in ("0.0.0.0", "198.51.100.7", "255.255.255.255"):
            with self.subTest(address=address):
                self.assertIsNone(re.fullmatch(self.cidr_pattern, f"{address}/0"))

    def test_accepts_restricted_cidrs(self):
        for prefix in range(1, 33):
            with self.subTest(prefix=prefix):
                self.assertIsNotNone(
                    re.fullmatch(self.cidr_pattern, f"198.51.100.7/{prefix}")
                )
        for invalid in ("999.0.0.1/32", "198.51.100.7/33", "::1/128"):
            self.assertIsNone(re.fullmatch(self.cidr_pattern, invalid))

    def test_instance_type_is_a_parameter_with_legacy_default(self):
        self.assertRegex(
            self.template,
            r"  InstanceType:\n    Type: String\n    Default: g6\.2xlarge",
        )
        self.assertIn("InstanceType: !Ref InstanceType", self.template)

    def test_output_identifies_successful_region(self):
        self.assertRegex(
            self.template,
            r"Outputs:\n  AWSRegion:\n    Description: [^\n]+\n    Value: !Ref AWS::Region",
        )


if __name__ == "__main__":
    unittest.main()
