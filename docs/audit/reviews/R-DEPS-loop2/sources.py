import hashlib,json,pathlib,urllib.request
root=pathlib.Path('/tmp/DEPS-review2/sources');root.mkdir(exist_ok=True)
urls={
 'archive-advisory.json':'https://vuln.go.dev/ID/GO-2026-6253.json',
 'crypto-advisory.json':'https://vuln.go.dev/ID/GO-2026-5932.json',
 'crypto-tags.txt':'https://proxy.golang.org/golang.org/x/crypto/@v/list',
 'testcontainers-release.json':'https://api.github.com/repos/testcontainers/testcontainers-go/releases/tags/v0.44.0',
 'archive-release.json':'https://api.github.com/repos/moby/go-archive/releases/tags/v0.3.0',
 'archive-previous-release.json':'https://api.github.com/repos/moby/go-archive/releases/tags/v0.2.1',
 'user-release.json':'https://api.github.com/repos/moby/sys/releases/tags/user/v0.4.1',
 'testcontainers.mod':'https://raw.githubusercontent.com/testcontainers/testcontainers-go/v0.44.0/go.mod',
 'archive.mod':'https://raw.githubusercontent.com/moby/go-archive/v0.3.0/go.mod',
 'user.mod':'https://raw.githubusercontent.com/moby/sys/user/v0.4.1/user/go.mod',
}
manifest=[]
for name,url in urls.items():
    body=urllib.request.urlopen(url,timeout=30).read();(root/name).write_bytes(body)
    manifest.append(dict(file=name,url=url,sha256=hashlib.sha256(body).hexdigest()))
    print(name, len(body),flush=True)
(root/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
