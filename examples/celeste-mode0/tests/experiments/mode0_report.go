//go:build ignore
package main
import("fmt";"os";"encoding/json";"github.com/drpaneas/mtile/internal/assetsync";"github.com/drpaneas/mtile/internal/sourcebundle";"github.com/drpaneas/mtile/internal/transpiler")
func main(){
 a,e:=assetsync.Load(os.Args[1]);if e!=nil{panic(e)}
 p,e:=assetsync.LoadProject(os.Args[1]);if e!=nil{panic(e)}
 entry:=p.Entry;if entry==""{entry="game.lua"};s,e:=sourcebundle.Resolve(os.Args[1],entry);if e!=nil{panic(e)}
 _,r,e:=transpiler.TranspileWithReport(s.Entry,s.Source,transpiler.Options{Renderer:transpiler.RendererMode0,SheetW:a.SheetW,SheetH:a.SheetH,Sheet:a.SheetROM,MapW:a.MapW,MapH:a.MapH,MapLayers:a.MapLayers,Map:a.MapROM,Flags:a.FlagROM,Palette:a.PaletteROM[:],NativeAudio:a.NativeAudio,IWRAMFunctions:p.IWRAMFunctions});if e!=nil{fmt.Println(s.RemapError(e));return};b,_:=json.MarshalIndent(r,"","  ");fmt.Println(string(b))
}
