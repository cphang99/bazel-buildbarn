workspace(name = "com_github_buildbarn_bb_remote_execution")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "bazel_gomock",
    sha256 = "eeed097c09e10238ca7ec06ac17eb5505eb7eb38d6282b284cb55c05e8ffc07f",
    strip_prefix = "bazel_gomock-ff6c20a9b6978c52b88b7a1e2e55b3b86e26685b",
    urls = ["https://github.com/jmhodges/bazel_gomock/archive/ff6c20a9b6978c52b88b7a1e2e55b3b86e26685b.tar.gz"],
)

http_archive(
    name = "bazel_toolchains",
    sha256 = "ee854b5de299138c1f4a2edb5573d22b21d975acfc7aa938f36d30b49ef97498",
    strip_prefix = "bazel-toolchains-37419a124bdb9af2fec5b99a973d359b6b899b61",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-toolchains/archive/37419a124bdb9af2fec5b99a973d359b6b899b61.tar.gz",
        "https://github.com/bazelbuild/bazel-toolchains/archive/37419a124bdb9af2fec5b99a973d359b6b899b61.tar.gz",
    ],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "aed1c249d4ec8f703edddf35cbe9dfaca0b5f5ea6e4cd9e83e99f3b0d1136c3d",
    strip_prefix = "rules_docker-0.7.0",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/v0.7.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "ade51a315fa17347e5c31201fdc55aa5ffb913377aa315dceb56ee9725e620ee",
    url = "https://github.com/bazelbuild/rules_go/releases/download/0.16.6/rules_go-0.16.6.tar.gz",
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "7949fc6cc17b5b191103e97481cf8889217263acf52e00b560683413af204fcb",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.16.0/bazel-gazelle-0.16.0.tar.gz"],
)

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "com_github_buildbarn_bb_browser",
    commit = "d28c09d2f146d2b3ad95a4aeef69f686d2512e8f",
    remote = "https://github.com/buildbarn/bb-browser.git",
)

git_repository(
    name = "com_github_buildbarn_bb_storage",
    commit = "e4d94e6d015f10a9fbb4d1f936e8392e76408449",
    remote = "https://github.com/buildbarn/bb-storage.git",
)

load("@io_bazel_rules_docker//repositories:repositories.bzl", container_repositories = "repositories")

container_repositories()

load("@io_bazel_rules_docker//container:container.bzl", "container_pull")

container_pull(
    name = "rbe_debian8_base",
    digest = "sha256:75ba06b78aa99e58cfb705378c4e3d6f0116052779d00628ecb73cd35b5ea77d",
    registry = "launcher.gcr.io",
    repository = "google/rbe-debian8",
)

container_pull(
    name = "rbe_ubuntu16_04_base",
    digest = "sha256:9bd8ba020af33edb5f11eff0af2f63b3bcb168cd6566d7b27c6685e717787928",
    registry = "launcher.gcr.io",
    repository = "google/rbe-ubuntu16-04",
)

load("@io_bazel_rules_go//go:def.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

gazelle_dependencies()

load("@com_github_buildbarn_bb_browser//:go_dependencies.bzl", "bb_browser_go_dependencies")

bb_browser_go_dependencies()

load("@com_github_buildbarn_bb_storage//:go_dependencies.bzl", "bb_storage_go_dependencies")

bb_storage_go_dependencies()
