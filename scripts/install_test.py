import hashlib
import io
import os
import pathlib
import shutil
import subprocess
import tarfile
import tempfile

script = pathlib.Path(__file__).resolve().with_name('install.sh')
with tempfile.TemporaryDirectory(prefix='prui-installer-test-') as root:
    root = pathlib.Path(root)
    platforms = [
        ('Darwin', 'arm64', 'darwin_arm64'),
        ('Darwin', 'x86_64', 'darwin_amd64'),
        ('Linux', 'aarch64', 'linux_arm64'),
        ('Linux', 'x86_64', 'linux_amd64'),
        ('FreeBSD', 'x86_64', None),
        ('Linux', 'riscv64', None),
    ]
    for system, machine, platform in platforms:
        for failure in (['', 'checksum', 'missing-checksum', 'download'] if platform else ['']):
            case = root / (system + machine + failure)
            home = case / 'home with spaces'
            mocks = case / 'mocks'
            downloads = case / 'downloads'
            for directory in (home / '.local/bin', mocks, downloads):
                directory.mkdir(parents=True)
            installed = home / '.local/bin/prui'
            installed.write_text('existing installation')
            if platform:
                archive = case / ('prui_1.2.3_' + platform + '.tar.gz')
                binary = b'#!/bin/sh\necho "prui 1.2.3"\n'
                with tarfile.open(archive, 'w:gz') as tar:
                    entry = tarfile.TarInfo('prui')
                    entry.size = len(binary)
                    entry.mode = 0o755
                    tar.addfile(entry, io.BytesIO(binary))
                digest = hashlib.sha256(archive.read_bytes()).hexdigest() if failure != 'checksum' else '0' * 64
                (case / 'checksums.txt').write_text('' if failure == 'missing-checksum' else digest + '  ' + archive.name + '\n')
            (mocks / 'uname').write_text('#!/bin/sh\ncase "$1" in -s) echo "$TEST_SYSTEM";; -m) echo "$TEST_MACHINE";; *) exit 1;; esac\n')
            (mocks / 'gh').write_text('''#!/bin/sh
set -eu
[ "$TEST_FAILURE" != download ] || exit 1
[ "$1 $2" = 'release download' ] || exit 1
shift 2
while [ "$#" -gt 0 ]; do
  case "$1" in
    --repo) [ "$2" = steve-mackinnon/prui ] || exit 1; shift 2;;
    --pattern) case "$2" in checksums.txt) ;; *) [ "$2" = "prui_*_${TEST_PLATFORM}.tar.gz" ] || exit 1;; esac; shift 2;;
    --dir) destination=$2; shift 2;;
    *) exit 1;;
  esac
done
cp "$TEST_CASE"/*.tar.gz "$TEST_CASE/checksums.txt" "$destination/"
''')
            if not shutil.which('sha256sum'):
                (mocks / 'sha256sum').write_text('#!/bin/sh\nexec shasum -a 256 "$@"\n')
            for mock in mocks.iterdir():
                mock.chmod(0o755)
            env = dict(
                os.environ,
                HOME=str(home), TMPDIR=str(downloads),
                PATH=str(mocks) + ':' + os.environ['PATH'],
                TEST_CASE=str(case), TEST_SYSTEM=system, TEST_MACHINE=machine,
                TEST_PLATFORM=platform or '', TEST_FAILURE=failure,
            )
            result = subprocess.run(['sh', str(script)], env=env, capture_output=True, text=True)
            success = platform and not failure
            assert (result.returncode == 0) == bool(success), (system, machine, failure, result.stdout, result.stderr)
            assert installed.read_text() == (binary.decode() if success else 'existing installation')
            assert not list(downloads.iterdir()), 'temporary downloads were not cleaned up'
            print('PASS', system, machine, failure or 'install')
