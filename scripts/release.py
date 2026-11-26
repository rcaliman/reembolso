#!/usr/bin/env python3
"""Gera binários Linux e um ZIP ARM64 reproduzível, sem dados da instalação."""
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]

def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

def build_release():
    # A lista explícita de arquivos impede a inclusão de bancos e anexos.
    with tempfile.TemporaryDirectory(prefix='.release-', dir=ROOT) as directory:
        stage = Path(directory)
        for arch, name in [('amd64', 'reembolso'), ('arm64', 'reembolso-pi')]:
            env = dict(os.environ, GOOS='linux', GOARCH=arch, CGO_ENABLED='0', GOAMD64='v1', GOARM64='v8.0')
            subprocess.run(['go', 'build', '-mod=readonly', '-trimpath', '-buildvcs=false',
                            '-ldflags=-buildid=', '-o', str(stage / name), '.'],
                           cwd=ROOT, env=env, check=True)
        files = [('reembolso', stage / 'reembolso-pi', 0o755),
                 ('iniciar.sh', ROOT / 'iniciar.sh', 0o755),
                 ('README.md', ROOT / 'README.md', 0o644)]
        checksums = ''.join(f'{digest(path)}  {name}\n' for name, path, _ in files)
        (stage / 'package-checksums').write_text(checksums, encoding='utf-8')
        files.append(('SHA256SUMS', stage / 'package-checksums', 0o644))
        with zipfile.ZipFile(stage / 'reembolso.zip', 'w', compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
            for name, path, mode in files:
                info = zipfile.ZipInfo('reembolso/' + name, date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (0o100000 | mode) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, path.read_bytes(), compresslevel=9)
        names = ['reembolso', 'reembolso-pi', 'reembolso.zip']
        (stage / 'SHA256SUMS').write_text(''.join(f'{digest(stage / name)}  {name}\n' for name in names), encoding='utf-8')
        # Só substitui os artefatos depois de todas as compilações concluídas.
        for name in names + ['SHA256SUMS']:
            os.replace(stage / name, ROOT / name)
    print('Gerados: reembolso, reembolso-pi, reembolso.zip e SHA256SUMS')

if __name__ == '__main__':
    build_release()
