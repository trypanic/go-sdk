import base64,difflib,io,pathlib,subprocess,sys,tarfile
sys.path.insert(0,'/tmp/DEPS-review2')
from run import OUT,ROOT,run
cases=[
 ('xp-json','httprequest/limits.go',[('\tif !l.exceeded {\n\t\t_, _ = io.Copy(io.Discard, l)\n\t}\n','')],'go test -race -count=1 -run TestChunkedJSONTrailingLimit ./httprequest','expected non-retriable oversize error'),
 ('xp-lifetime','messaging/messaging.go',[('\treturn &PubSub{\n\t\tpublisher:', '\tpubsub := &PubSub{\n\t\tpublisher:'),('\t\tlog:        build.log,\n\t}, nil','\t\tlog:        build.log,\n\t}\n\tpubsub.lifeOnce.Do(func() { pubsub.life, pubsub.stop = context.WithCancel(ctx) })\n\treturn pubsub, nil')],'go test -race -count=1 -run TestSubscribeOutlivesConstructionContext ./messaging','subscription inherited construction cancellation'),
]
for name,path,replacements,command,needle in cases:
    p=OUT/name;p.mkdir()
    with tarfile.open(fileobj=io.BytesIO(subprocess.check_output(['git','archive','ddf7afd'],cwd=ROOT))) as t:t.extractall(p,filter='data')
    f=p/path;before=f.read_text();after=before
    for old,new in replacements:
        assert after.count(old)==1
        after=after.replace(old,new)
    f.write_text(after)
    patch=''.join(difflib.unified_diff(before.splitlines(True),after.splitlines(True),fromfile='a/'+path,tofile='b/'+path))
    (OUT/(name+'.patch.b64')).write_text(base64.b64encode(patch.encode()).decode()+'\n')
    assert run(name,command,cwd=p,expected=1)==1
    assert needle in (OUT/(name+'.log')).read_text()
