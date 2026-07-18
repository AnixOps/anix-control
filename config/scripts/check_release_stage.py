#!/usr/bin/env python3
"""Validate product release-stage scope independently from the Go module major."""

from __future__ import annotations

import argparse
import json
import re
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any


TAG_PATTERN = re.compile(
    r"^v(?P<major>0|[1-9][0-9]*)\.(?P<minor>0|[1-9][0-9]*)\.(?P<patch>0|[1-9][0-9]*)"
    r"(?:-(?P<channel>alpha|beta|rc)(?:\.(?P<sequence>[1-9][0-9]*))?)?$"
)
VERSION_PATTERN = re.compile(r"^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$")
MODULE_PATTERN = re.compile(r"^module\s+(?P<path>\S+)\s*$", re.MULTILINE)
MODULE_MAJOR_PATTERN = re.compile(r"/v(?P<major>[1-9][0-9]*)$")


class StageContractError(ValueError):
    """Raised when a release candidate does not meet its declared stage."""


@dataclass(frozen=True)
class ReleaseTag:
    raw: str
    major: int
    minor: int
    patch: int
    channel: str | None

    @property
    def product_version(self) -> str:
        return f"{self.major}.{self.minor}.{self.patch}"

    @property
    def stage_id(self) -> str:
        return f"{self.major}.{self.minor}"


@dataclass(frozen=True)
class StageDecision:
    tag: str
    classification: str
    release_eligible: bool
    stage_id: str | None
    product_version: str
    module_path: str | None = None
    module_major: int | None = None
    package_ids: tuple[str, ...] = ()


def parse_tag(tag: str) -> ReleaseTag:
    match = TAG_PATTERN.fullmatch(tag)
    if not match:
        raise StageContractError(f"unsupported release tag: {tag}")
    return ReleaseTag(
        raw=tag,
        major=int(match.group("major")),
        minor=int(match.group("minor")),
        patch=int(match.group("patch")),
        channel=match.group("channel"),
    )


def parse_version(value: str, label: str) -> tuple[int, int, int]:
    match = VERSION_PATTERN.fullmatch(value)
    if not match:
        raise StageContractError(f"{label} must be a stable semantic version, found {value!r}")
    return tuple(int(part) for part in match.groups())


def read_contract(path: Path) -> dict[str, Any]:
    try:
        decoded = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError as error:
        raise StageContractError(f"release stage declaration not found: {path}") from error
    except json.JSONDecodeError as error:
        raise StageContractError(f"release stage declaration is not valid JSON: {path}: {error}") from error
    if not isinstance(decoded, dict):
        raise StageContractError("release stage declaration root must be an object")
    validate_contract_shape(decoded)
    return decoded


def validate_contract_shape(contract: dict[str, Any]) -> None:
    if contract.get("schema_version") != 1:
        raise StageContractError("unsupported release stage declaration schema_version")

    product = contract.get("product")
    if not isinstance(product, dict):
        raise StageContractError("release stage declaration product must be an object")
    if not isinstance(product.get("go_module_path"), str) or not product["go_module_path"]:
        raise StageContractError("release stage declaration product.go_module_path is required")
    if not isinstance(product.get("go_module_major"), int) or product["go_module_major"] < 1:
        raise StageContractError("release stage declaration product.go_module_major must be positive")

    legacy_tags = contract.get("legacy_preview_tags")
    if not isinstance(legacy_tags, list) or not all(isinstance(tag, str) for tag in legacy_tags):
        raise StageContractError("release stage declaration legacy_preview_tags must be a string list")
    if len(set(legacy_tags)) != len(legacy_tags):
        raise StageContractError("release stage declaration legacy_preview_tags contains duplicates")
    for tag in legacy_tags:
        parsed = parse_tag(tag)
        if parsed.channel is None:
            raise StageContractError(f"legacy preview tag must be a prerelease: {tag}")
    legacy_package_ids = contract.get("legacy_preview_package_ids")
    if not isinstance(legacy_package_ids, list) or not legacy_package_ids:
        raise StageContractError("release stage declaration legacy_preview_package_ids must be a non-empty list")
    if not all(isinstance(package_id, str) and package_id for package_id in legacy_package_ids):
        raise StageContractError("release stage declaration legacy_preview_package_ids must contain package ids")
    if len(set(legacy_package_ids)) != len(legacy_package_ids):
        raise StageContractError("release stage declaration legacy_preview_package_ids contains duplicates")

    configuration = contract.get("configuration_contract")
    if not isinstance(configuration, dict):
        raise StageContractError("release stage declaration configuration_contract must be an object")
    for profile_key in ("safe_default_templates", "explicit_canary_templates"):
        profiles = configuration.get(profile_key)
        if not isinstance(profiles, list) or not profiles:
            raise StageContractError(f"configuration_contract.{profile_key} must be a non-empty list")
        for profile in profiles:
            if not isinstance(profile, dict) or not isinstance(profile.get("path"), str):
                raise StageContractError(f"configuration_contract.{profile_key} entries require path")
            plugins = profile.get("plugins")
            if not isinstance(plugins, dict):
                raise StageContractError(f"configuration_contract.{profile_key} entries require plugins")
            for key in ("control_execution_enabled", "dispatch_enabled", "topology_execution_enabled"):
                if not isinstance(plugins.get(key), bool):
                    raise StageContractError(
                        f"configuration_contract.{profile_key} {profile['path']} requires boolean plugins.{key}"
                    )

    stages = contract.get("stages")
    if not isinstance(stages, list) or not stages:
        raise StageContractError("release stage declaration stages must be a non-empty list")
    stage_ids: set[str] = set()
    for stage in stages:
        if not isinstance(stage, dict):
            raise StageContractError("release stage declaration stage entries must be objects")
        stage_id = stage.get("id")
        product_version = stage.get("product_version")
        if not isinstance(stage_id, str) or not re.fullmatch(r"[1-9][0-9]*\.[0-9]+", stage_id):
            raise StageContractError("release stage declaration stage id must be major.minor")
        if stage_id in stage_ids:
            raise StageContractError(f"release stage declaration has duplicate stage id: {stage_id}")
        stage_ids.add(stage_id)
        version = parse_version(str(product_version), f"stage {stage_id} product_version")
        if stage_id != f"{version[0]}.{version[1]}" or version[2] != 0:
            raise StageContractError(f"stage {stage_id} product_version must be its .0 release")
        packages = stage.get("required_packages")
        if not isinstance(packages, list) or not packages:
            raise StageContractError(f"stage {stage_id} required_packages must be a non-empty list")
        package_ids: set[str] = set()
        for package in packages:
            if not isinstance(package, dict):
                raise StageContractError(f"stage {stage_id} package entries must be objects")
            package_id = package.get("id")
            if not isinstance(package_id, str) or not package_id:
                raise StageContractError(f"stage {stage_id} package id is required")
            if package_id in package_ids:
                raise StageContractError(f"stage {stage_id} has duplicate required package: {package_id}")
            package_ids.add(package_id)
            for key in ("manifest", "minimum_version"):
                if not isinstance(package.get(key), str) or not package[key]:
                    raise StageContractError(f"stage {stage_id} package {package_id} requires {key}")
            parse_version(package["minimum_version"], f"stage {stage_id} package {package_id} minimum_version")
            targets = package.get("required_targets")
            if not isinstance(targets, list) or not targets or not all(isinstance(target, str) for target in targets):
                raise StageContractError(f"stage {stage_id} package {package_id} requires target names")
            if not isinstance(package.get("requires_webui"), bool):
                raise StageContractError(f"stage {stage_id} package {package_id} requires boolean requires_webui")
        out_of_scope = stage.get("out_of_scope_package_ids")
        if not isinstance(out_of_scope, list) or not all(isinstance(package_id, str) for package_id in out_of_scope):
            raise StageContractError(f"stage {stage_id} out_of_scope_package_ids must be a string list")
        overlap = package_ids.intersection(out_of_scope)
        if overlap:
            raise StageContractError(
                f"stage {stage_id} packages cannot be both required and out of scope: {', '.join(sorted(overlap))}"
            )
        manifest_api_version = stage.get("manifest_api_version")
        if manifest_api_version is not None and manifest_api_version not in {"v1", "v2"}:
            raise StageContractError(f"stage {stage_id} manifest_api_version must be v1 or v2")
        if stage_id == "4.0" and manifest_api_version != "v2":
            raise StageContractError("stage 4.0 must require manifest_api_version v2")


def stages_by_id(contract: dict[str, Any]) -> dict[str, dict[str, Any]]:
    return {stage["id"]: stage for stage in contract["stages"]}


def module_identity(repo_root: Path, contract: dict[str, Any]) -> tuple[str, int]:
    go_mod = repo_root / "go.mod"
    if not go_mod.is_file():
        raise StageContractError(f"go.mod is required to validate product/module version separation: {go_mod}")
    match = MODULE_PATTERN.search(go_mod.read_text(encoding="utf-8"))
    if not match:
        raise StageContractError("go.mod module declaration is missing")
    module_path = match.group("path")
    expected_path = contract["product"]["go_module_path"]
    if module_path != expected_path:
        raise StageContractError(f"Go module path mismatch: expected {expected_path}, found {module_path}")
    major_match = MODULE_MAJOR_PATTERN.search(module_path)
    if not major_match:
        raise StageContractError(f"Go module path has no /vN suffix: {module_path}")
    module_major = int(major_match.group("major"))
    expected_major = contract["product"]["go_module_major"]
    if module_major != expected_major:
        raise StageContractError(f"Go module major mismatch: expected {expected_major}, found {module_major}")
    return module_path, module_major


def plugin_boolean_values(path: Path) -> dict[str, bool]:
    if not path.is_file():
        raise StageContractError(f"configuration template is missing: {path}")
    values: dict[str, bool] = {}
    plugins_indent: int | None = None
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        without_comment = raw_line.split("#", 1)[0]
        if not without_comment.strip():
            continue
        indentation = len(without_comment) - len(without_comment.lstrip(" "))
        stripped = without_comment.strip()
        if plugins_indent is None:
            if stripped == "plugins:":
                plugins_indent = indentation
            continue
        if indentation <= plugins_indent:
            break
        match = re.fullmatch(r"([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(true|false)\s*", stripped)
        if match:
            values[match.group(1)] = match.group(2) == "true"
    return values


def validate_configuration_contract(repo_root: Path, contract: dict[str, Any]) -> None:
    configuration = contract["configuration_contract"]
    for profile_key in ("safe_default_templates", "explicit_canary_templates"):
        for profile in configuration[profile_key]:
            path = repo_root / profile["path"]
            actual = plugin_boolean_values(path)
            for key, expected in profile["plugins"].items():
                found = actual.get(key)
                if found is None:
                    raise StageContractError(f"{profile['path']} is missing plugins.{key}")
                if found != expected:
                    raise StageContractError(
                        f"{profile['path']} plugins.{key} mismatch: expected {str(expected).lower()}, "
                        f"found {str(found).lower()}"
                    )


def semantic_version_at_least(actual: str, minimum: str) -> bool:
    return parse_version(actual, "package manifest version") >= parse_version(minimum, "package minimum_version")


def validate_v2_host_metadata(manifest: dict[str, Any], package_id: str) -> None:
    if manifest.get("api_version") != "v2":
        raise StageContractError(f"package {package_id} api_version must be v2")

    def require_file(field: str, path_key: str) -> None:
        value = manifest.get(field)
        if not isinstance(value, dict):
            raise StageContractError(f"package {package_id} {field} is required")
        if not isinstance(value.get(path_key), str) or not value[path_key]:
            raise StageContractError(f"package {package_id} {field}.{path_key} is required")
        if not isinstance(value.get("sha256"), str) or not value["sha256"]:
            raise StageContractError(f"package {package_id} {field}.sha256 is required")

    require_file("control_entrypoint", "path")
    require_file("migrations", "index")
    require_file("compatibility_routes", "path")
    route_contract_digest = manifest.get("route_contract_digest")
    if not isinstance(route_contract_digest, str) or not route_contract_digest:
        raise StageContractError(f"package {package_id} route_contract_digest is required")

    targets = manifest.get("targets")
    if isinstance(targets, list) and "agent" in targets:
        require_file("agent_entrypoint", "path")
        if not isinstance(manifest.get("runtime_api_version"), str) or not manifest["runtime_api_version"]:
            raise StageContractError(f"package {package_id} runtime_api_version is required for the agent target")


def validate_package_requirement(repo_root: Path, requirement: dict[str, Any], manifest_api_version: str | None = None) -> None:
    manifest_path = repo_root / requirement["manifest"]
    if not manifest_path.is_file():
        raise StageContractError(f"required package manifest is missing: {requirement['id']} ({manifest_path})")
    try:
        manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as error:
        raise StageContractError(f"required package manifest is invalid JSON: {manifest_path}: {error}") from error
    if manifest.get("id") != requirement["id"]:
        raise StageContractError(
            f"package manifest id mismatch at {manifest_path}: expected {requirement['id']}, found {manifest.get('id')!r}"
        )
    version = manifest.get("version")
    if not isinstance(version, str) or not semantic_version_at_least(version, requirement["minimum_version"]):
        raise StageContractError(
            f"package {requirement['id']} version must be at least {requirement['minimum_version']}, found {version!r}"
        )
    targets = manifest.get("targets")
    if not isinstance(targets, list) or not set(requirement["required_targets"]).issubset(set(targets)):
        raise StageContractError(
            f"package {requirement['id']} targets must include {', '.join(requirement['required_targets'])}"
        )
    if manifest_api_version == "v2":
        validate_v2_host_metadata(manifest, requirement["id"])
    if not requirement["requires_webui"]:
        return
    webui = manifest.get("webui")
    if not isinstance(webui, dict):
        raise StageContractError(f"package {requirement['id']} must declare a WebUI bundle")
    bundle = webui.get("bundle")
    if not isinstance(bundle, dict) or not isinstance(bundle.get("path"), str) or not bundle["path"]:
        raise StageContractError(f"package {requirement['id']} WebUI bundle path is required")
    if not isinstance(bundle.get("sha256"), str) or not bundle["sha256"]:
        raise StageContractError(f"package {requirement['id']} WebUI bundle digest is required")
    bundle_path = manifest_path.parent / bundle["path"]
    if not bundle_path.is_file():
        raise StageContractError(f"package {requirement['id']} WebUI bundle is missing: {bundle_path}")
    for key in ("menus", "routes"):
        if not isinstance(webui.get(key), list) or not webui[key]:
            raise StageContractError(f"package {requirement['id']} WebUI {key} are required")


def validate_stage(repo_root: Path, contract: dict[str, Any], tag: str) -> StageDecision:
    parsed = parse_tag(tag)
    if tag in set(contract["legacy_preview_tags"]):
        return StageDecision(
            tag=tag,
            classification="historical-preview",
            release_eligible=False,
            stage_id=None,
            product_version=parsed.product_version,
        )

    configured_stages = stages_by_id(contract)
    stage = configured_stages.get(parsed.stage_id)
    if stage is None:
        raise StageContractError(f"no release-stage contract is declared for product {parsed.stage_id}")
    module_path, module_major = module_identity(repo_root, contract)
    validate_configuration_contract(repo_root, contract)
    for requirement in stage["required_packages"]:
        validate_package_requirement(repo_root, requirement, stage.get("manifest_api_version"))
    return StageDecision(
        tag=tag,
        classification="product-stage",
        release_eligible=True,
        stage_id=stage["id"],
        product_version=parsed.product_version,
        module_path=module_path,
        module_major=module_major,
        package_ids=tuple(requirement["id"] for requirement in stage["required_packages"]),
    )


def write_github_output(path: Path, decision: StageDecision) -> None:
    package_ids = ",".join(decision.package_ids)
    values = {
        "release_stage_classification": decision.classification,
        "release_eligible": str(decision.release_eligible).lower(),
        "product_stage": decision.stage_id or decision.classification,
        "release_package_ids": package_ids,
        "release_package_scope": f",{package_ids}," if package_ids else ",",
    }
    with path.open("a", encoding="utf-8") as output:
        for key, value in values.items():
            output.write(f"{key}={value}\n")


def write_fixture(root: Path, contract: dict[str, Any]) -> Path:
    declaration_path = root / "config/scripts/release-stage-contract.json"
    declaration_path.parent.mkdir(parents=True, exist_ok=True)
    declaration_path.write_text(json.dumps(contract, indent=2), encoding="utf-8")
    (root / "go.mod").write_text("module github.com/AnixOps/anix-control/v4\n", encoding="utf-8")

    template_values = {
        "config/config.yaml.example": (False, False, False),
        "config/config.prod.yaml": (False, False, False),
        "config/config.dev.yaml.example": (True, True, False),
    }
    for relative_path, values in template_values.items():
        path = root / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(
            "plugins:\n"
            f"  control_execution_enabled: {str(values[0]).lower()}\n"
            f"  dispatch_enabled: {str(values[1]).lower()}\n"
            f"  topology_execution_enabled: {str(values[2]).lower()}\n",
            encoding="utf-8",
        )

    packages = {
        "machine-telemetry": "1.1.0",
        "nftables-forward": "1.2.0",
        "gost-mesh": "1.0.0",
        "nat-egress": "1.0.0",
    }
    for package_id, version in packages.items():
        path = root / "packages" / package_id / "manifest.template.json"
        path.parent.mkdir(parents=True, exist_ok=True)
        webui_bundle = path.parent / "webui/index.mjs"
        webui_bundle.parent.mkdir(parents=True, exist_ok=True)
        webui_bundle.write_text("export default {};\n", encoding="utf-8")
        path.write_text(
            json.dumps(
                {
                    "id": package_id,
                    "version": version,
                    "targets": ["control", "agent"],
                    "webui": {
                        "bundle": {"path": "webui/index.mjs", "sha256": "fixture-digest"},
                        "menus": [{"id": f"{package_id}.main"}],
                        "routes": [{"id": f"{package_id}.main"}],
                    },
                }
            ),
            encoding="utf-8",
        )
    return declaration_path


def expect_failure(label: str, callback: Any) -> None:
    try:
        callback()
    except StageContractError:
        return
    raise AssertionError(f"self-test expected failure: {label}")


def run_self_test(declaration_path: Path) -> None:
    contract = read_contract(declaration_path)
    stage_31 = stages_by_id(contract)["3.1"]
    required_31 = [package["id"] for package in stage_31["required_packages"]]
    if required_31 != ["machine-telemetry"]:
        raise AssertionError("3.1 must require only machine-telemetry")
    future_31 = {"nftables-forward", "gost-mesh", "nat-egress", "wireguard", "protocol-runtime"}
    if future_31.intersection(required_31):
        raise AssertionError("3.1 must not require later-stage forwarding packages")

    with tempfile.TemporaryDirectory(prefix="anix-release-stage-") as temporary_root:
        root = Path(temporary_root)
        fixture_declaration = write_fixture(root, contract)
        fixture_contract = read_contract(fixture_declaration)

        decision = validate_stage(root, fixture_contract, "v3.1.0")
        if (
            decision.classification != "product-stage"
            or not decision.release_eligible
            or decision.stage_id != "3.1"
        ):
            raise AssertionError("3.1 product tag was not resolved to the 3.1 stage")
        if decision.module_major != 4 or decision.module_path != "github.com/AnixOps/anix-control/v4":
            raise AssertionError("product 3.1 must remain valid with Go module /v4")
        github_output = root / "github-output.txt"
        write_github_output(github_output, decision)
        output_values = dict(
            line.split("=", 1)
            for line in github_output.read_text(encoding="utf-8").splitlines()
            if "=" in line
        )
        if output_values.get("release_package_ids") != "machine-telemetry":
            raise AssertionError("3.1 GitHub release output must contain only machine-telemetry")
        if output_values.get("release_package_scope") != ",machine-telemetry,":
            raise AssertionError("3.1 GitHub release output has an invalid package scope")
        if output_values.get("release_eligible") != "true":
            raise AssertionError("3.1 GitHub release output must be publishable")

        historical = validate_stage(root, fixture_contract, "v4.0.0-alpha.7")
        if historical.classification != "historical-preview" or historical.release_eligible:
            raise AssertionError("historical v4 alpha tag must be audit-only")
        if historical.package_ids:
            raise AssertionError("historical v4 alpha tag must not declare release package assets")
        historical_output = root / "historical-github-output.txt"
        write_github_output(historical_output, historical)
        historical_values = dict(
            line.split("=", 1)
            for line in historical_output.read_text(encoding="utf-8").splitlines()
            if "=" in line
        )
        if historical_values.get("release_eligible") != "false":
            raise AssertionError("historical v4 alpha GitHub output must be audit-only")
        if historical_values.get("release_package_ids"):
            raise AssertionError("historical v4 alpha GitHub output must not select package assets")
        expect_failure("undeclared pre-stage v2 tag", lambda: validate_stage(root, fixture_contract, "v2.5.0"))
        expect_failure("undeclared pre-stage v3 tag", lambda: validate_stage(root, fixture_contract, "v3.0.0"))

        default_template = root / "config/config.yaml.example"
        default_template.write_text(
            "plugins:\n  control_execution_enabled: true\n  dispatch_enabled: false\n  topology_execution_enabled: false\n",
            encoding="utf-8",
        )
        expect_failure("unsafe normal template", lambda: validate_stage(root, fixture_contract, "v3.1.0"))

        write_fixture(root, fixture_contract)
        telemetry_manifest = root / "packages/machine-telemetry/manifest.template.json"
        telemetry_manifest.write_text(
            json.dumps({"id": "not-machine-telemetry", "version": "1.1.0", "targets": ["control", "agent"]}),
            encoding="utf-8",
        )
        expect_failure("tampered machine-telemetry manifest", lambda: validate_stage(root, fixture_contract, "v3.1.0"))

        write_fixture(root, fixture_contract)
        expect_failure("undeclared future v4 alpha", lambda: validate_stage(root, fixture_contract, "v4.0.0-alpha.8"))
        expect_failure("invalid tag", lambda: validate_stage(root, fixture_contract, "v3.1"))

    print("release stage contract self-test passed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", help="Product release tag, including the leading v")
    parser.add_argument(
        "--repo-root",
        type=Path,
        default=Path(__file__).resolve().parents[2],
        help="Repository root to inspect",
    )
    parser.add_argument(
        "--declaration",
        type=Path,
        default=Path(__file__).with_name("release-stage-contract.json"),
        help="Path to the data-driven release stage declaration",
    )
    parser.add_argument(
        "--github-output",
        type=Path,
        help="Append stage classification and package scope to a GitHub Actions output file",
    )
    parser.add_argument("--self-test", action="store_true", help="Run isolated contract checker self-tests")
    args = parser.parse_args()

    try:
        if args.self_test:
            run_self_test(args.declaration.resolve())
            return 0
        if not args.tag:
            parser.error("--tag is required unless --self-test is used")
        contract = read_contract(args.declaration.resolve())
        decision = validate_stage(args.repo_root.resolve(), contract, args.tag)
        if args.github_output:
            write_github_output(args.github_output, decision)
    except StageContractError as error:
        print(f"error: {error}")
        return 1

    if decision.release_eligible:
        print(
            f"release stage accepted: {decision.tag} -> {decision.stage_id} "
            f"(product {decision.product_version}; Go module v{decision.module_major}; "
            f"packages: {', '.join(decision.package_ids)})"
        )
    else:
        print(f"release stage audit accepted: {decision.tag} ({decision.classification}; publish disabled)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
