package nasobserver

import (
 "context"
 "crypto/ed25519"
 "crypto/rand"
 "os"
 "path/filepath"
 "testing"
 "time"
)

func TestCriticalEvidenceFailClosed(t *testing.T) {
 now:=time.Now().UTC()
 signal:=func(id string,v bool) AttributedSignal{return AttributedSignal{SourceIdentity:id,Known:true,Value:v,ObservedAt:now}}
 ids:=CriticalSourceIdentities{Jellyfin:"jellyfin",FFmpeg:"ffmpeg",Worker:"worker",Storage:"storage"}
 evidence:=CriticalEvidence{JellyfinPlayback:signal("jellyfin",false),FFmpegActivity:signal("ffmpeg",false),WorkerActivity:signal("worker",false),StorageHealthy:signal("storage",true)}
 check:=func(want string){t.Helper();got:=GuardFromAttributedEvidence(evidence,ids,"guard",now,30*time.Second);if got.Status!=want{t.Fatalf("got %s want %s",got.Status,want)}}
 check("allow")
 evidence.JellyfinPlayback.Value=true;check("pressure")
 evidence.JellyfinPlayback.Value=false;evidence.WorkerActivity.Known=false;check("unavailable")
 evidence.WorkerActivity.Known=true;evidence.WorkerActivity.SourceIdentity="spoof";check("unavailable")
 evidence.WorkerActivity=signal("worker",false);evidence.StorageHealthy.ObservedAt=now.Add(-2*time.Minute);check("unavailable")
}

func TestJellyfinSessionParserConservative(t *testing.T) {
 for _,tc:=range []struct{body string;busy bool;err bool}{
  {`[]`,false,false},
  {`[{"NowPlayingItem":null}]`,false,false},
  {`[{"NowPlayingItem":{"Name":"Movie"}}]`,true,false},
  {`[{"NowPlayingItem":{"Name":"Movie"},"PlayState":{"IsPaused":true}}]`,true,false},
  {`[{"nowPlayingItem":{"Name":"Movie"}}]`,false,true},
  {`[{"NowPlayingItem":false}]`,false,true},
  {`[{"NowPlayingItem":null,"NowPlayingItem":{"Name":"X"}}]`,false,true},
  {`{}`,false,true}, {`null`,false,true},
 }{
  busy,err:=ParseJellyfinSessions([]byte(tc.body));if (err!=nil)!=tc.err||(!tc.err&&busy!=tc.busy){t.Fatalf("%q => %v %v",tc.body,busy,err)}
 }
}

func TestAtomicSignedSnapshotPublicationFixture(t *testing.T) {
 dir:=t.TempDir();target:=filepath.Join(dir,"observation.json")
 pub,priv,err:=ed25519.GenerateKey(rand.Reader);if err!=nil{t.Fatal(err)}
 _=pub
 now:=time.Now().UTC()
 payload:=Payload{SchemaVersion:1,CollectorIdentity:"collector",CapacityDomain:"fixture-nas",
 Host:Host{SourceIdentity:"proc",ObservedAt:now,TotalMemoryBytes:24<<30,AvailableMemoryBytes:12<<30,SwapKnown:true,SwapObservedAt:now,SwapTotalBytes:8<<30,SwapAvailableBytes:6<<30,CPUKnown:true,CPUObservedAt:now,CPUUsagePercent:22},
 Provider:Provider{SourceIdentity:"ollama",Available:true,ObservedAt:now},
 Critical:Critical{SourceIdentity:"critical",Status:"unavailable",ObservedAt:now}}
 data,err:=SignSnapshot(priv,payload);if err!=nil{t.Fatal(err)}
 if err:=PublishSignedSnapshotAtomically(context.Background(),target,data);err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile(target);if err!=nil||string(original)!=string(data){t.Fatal("publication mismatch")}
 for _,invalid:=range [][]byte{nil,[]byte("{}"),[]byte("not-json")} {
  if err:=PublishSignedSnapshotAtomically(context.Background(),target,invalid);err==nil{t.Fatal("invalid envelope accepted")}
  after,err:=os.ReadFile(target);if err!=nil||string(after)!=string(original){t.Fatal("failed publication modified original")}
 }
 other:=filepath.Join(dir,"elsewhere");if err:=os.WriteFile(other,[]byte("protected"),0600);err!=nil{t.Fatal(err)}
 link:=filepath.Join(dir,"symlink");if err:=os.Symlink(other,link);err!=nil{t.Fatal(err)}
 if err:=PublishSignedSnapshotAtomically(context.Background(),link,data);err==nil{t.Fatal("symlink destination accepted")}
 untouched,err:=os.ReadFile(other);if err!=nil||string(untouched)!="protected"{t.Fatal("symlink target modified")}
}
