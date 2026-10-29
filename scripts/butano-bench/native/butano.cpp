// Generated assets and compile-time workload constants are supplied by run.py.
#include "bench_assets.h"
#include "bn_core.h"
#include "bn_dmg_music.h"
#include "bn_dmg_music_item.h"
#include "bn_dmg_music_type.h"
#include "bn_sprite_ptr.h"
#include "bn_sprite_tiles_ptr.h"
#include "bn_regular_bg_ptr.h"
#include "bn_regular_bg_map_ptr.h"
#include "bn_regular_bg_tiles_ptr.h"
#include "bn_bg_palettes.h"
#include "bn_sprite_palettes.h"
#include "bn_sprite_palette_ptr.h"
#include "bn_memory.h"
#include "bn_vector.h"
#include "bn_optional.h"
#include "bn_sprite_text_generator.h"
#include "bn_sprite_font.h"
#include "bn_string.h"
#include "bn_bg_palette_ptr.h"
#include "bn_palette_bitmap_bg_ptr.h"
#include "bn_palette_bitmap_bg_painter.h"

extern "C" void bench_start();
extern "C" unsigned bench_finish();

static volatile uint32_t* const mb = reinterpret_cast<volatile uint32_t*>(0x0203fe00);
static int32_t px[ENTITIES ? ENTITIES:1],py[ENTITIES ? ENTITIES:1],vx[ENTITIES ? ENTITIES:1],vy[ENTITIES ? ENTITIES:1];
static unsigned checksum;
static uint32_t max_cycles, missed;

static void publish(unsigned tick,unsigned cycles) {
 mb[2]=tick*2+1;
 mb[0]=0x44554c47;mb[1]=0x02000001;
 mb[3]=tick;mb[4]=missed;mb[5]=cycles;mb[6]=max_cycles;
 mb[17]=2; // watch count at offset 68; remaining 3 bytes zero
 volatile char* names=reinterpret_cast<volatile char*>(0x0203fe00+72);
 const char* n="tick";for(int i=0;i<4;++i)names[i]=n[i];names[4]=0;
 mb[22]=1;mb[23]=tick<<16;
 names+=24;n="checksum";for(int i=0;i<8;++i)names[i]=n[i];names[8]=0;
 mb[28]=1;mb[29]=checksum<<16;
 mb[114]=bn::memory::used_static_ewram();
 mb[115]=bn::memory::used_static_iwram();
 mb[116]=bn::memory::used_stack_iwram();
 mb[117]=bn::memory::used_alloc_ewram();
 mb[2]=tick*2+2;
}

int main() {
 bn::core::init();bn::core::set_skip_frames(1);
 bn::bg_palettes::set_transparent_color(colors[1]);
 if constexpr(KIND==12){bn::dmg_music::play(bn::dmg_music_item(music_data[0],bn::dmg_music_type::VGM));bn::dmg_music::set_master_volume(bn::dmg_music_master_volume::FULL);}
 bn::vector<bn::sprite_ptr,127> sprites;
 bn::vector<bn::sprite_tiles_ptr,4> animation_tiles;
 if constexpr(KIND==2)for(int i=0;i<4;++i)animation_tiles.push_back(sprite_item.tiles_item().create_tiles(i));
 bn::vector<bn::regular_bg_ptr,4> backgrounds;
 bn::optional<bn::palette_bitmap_bg_ptr> bitmap;
 bn::optional<bn::regular_bg_map_ptr> mutable_map;
 bn::optional<bn::regular_bg_tiles_ptr> dirty_tiles;
 bn::optional<bn::bg_palette_ptr> palette;
 bn::optional<bn::sprite_text_generator> text;
 bn::vector<bn::sprite_ptr,64> text_sprites;
 if constexpr(KIND==13) { bitmap=bn::palette_bitmap_bg_ptr::create(bn::bg_palette_item(colors,bn::bpp_mode::BPP_8)); }
 for(int i=0;i<LAYERS;++i) {
  backgrounds.push_back(bg_items[i].create_bg(0,0));
  backgrounds.back().set_top_left_position(0,0);
  backgrounds.back().set_priority(3-i);
 }
 if constexpr(KIND==6)mutable_map=backgrounds[0].map();
 if constexpr(KIND==7)dirty_tiles=backgrounds[0].tiles();
 if constexpr(KIND==9)palette=backgrounds[0].palette();
 if constexpr(KIND==8 || KIND==14 || KIND==15) {
  text=bn::sprite_text_generator(font);
  text->set_one_sprite_per_character(true);
  text->set_left_alignment();
  text->set_bg_priority(0);
  if constexpr(KIND==8)text->generate_top_left(8,8,HUD,text_sprites);
 }
 for(int i=0;i<OBJECTS;++i) {
  sprites.push_back(sprite_item.create_sprite(0,0));
  sprites.back().set_bg_priority(0);
  sprites.back().put_above();
 }
 for(int i=0;i<ENTITIES;++i) {
  px[i]=(8+(i*13)%220)<<16;py[i]=(8+(i*17)%136)<<16;
  vx[i]=((i%7)-3)*8192;vy[i]=((i%5)-2)*8192;
 }
 for(unsigned tick=1;;++tick) {
  bench_start();
  const int frame=int(tick%120);
  checksum=0;
  if constexpr(KIND==10) {
   for(int i=0;i<ENTITIES;++i) {
    auto x=px[i],y=py[i],dx=vx[i],dy=vy[i]+4096;
    if constexpr(DRAG){dx=int32_t((int64_t(dx)*65280)>>16);dy=int32_t((int64_t(dy)*65280)>>16);}
    x+=dx;y+=dy;
    if(x<4*65536){x=4*65536;dx=dx<0?-dx:dx;}else if(x>235*65536){x=235*65536;dx=dx>0?-dx:dx;}
    if(y<4*65536){y=4*65536;dy=dy<0?-dy:dy;}else if(y>155*65536){y=155*65536;dy=dy>0?-dy:dy;}
    px[i]=x;py[i]=y;vx[i]=dx;vy[i]=dy;
    checksum=(checksum+unsigned(x>>16)+unsigned(y>>16))%16001;
   }
  }
  if constexpr(KIND==11) {
   for(int i=0;i<ENTITIES;++i) {
    int x=(i*7+frame)%32,y=(i*11+frame)%32;
    checksum=(checksum+map_cells[0][y*32+x])%16001;
   }
  }
  for(int i=0;i<LAYERS;++i) {
   int cam=KIND==4 || KIND==5 || KIND==14 ? frame*(i+1)*SCROLL_SPEED:0;
   int cam_y=KIND==4 || KIND==5 || KIND==14 ? frame*VERTICAL_SPEED:0;
   backgrounds[i].set_top_left_position(-cam,-cam_y);
  }
  if constexpr(KIND==6) {
   for(int i=0;i<MUTATIONS;++i)const_cast<uint16_t*>(map_cells[0])[i]=2+frame%2;
   mutable_map->reload_cells_ref();
  }
  if constexpr(KIND==7) {
   for(int i=0;i<MUTATIONS;++i) {
    int tile=SPREAD ? i+1:1, p=SPREAD ? 0:i;
    auto bytes=reinterpret_cast<uint8_t*>(mutable_tiles);
    bytes[tile*64+p]=2+(frame%2)*3;
   }
   for(int i=0;i<(SPREAD ? MUTATIONS:1);++i) {
    int tile=i+1;
    dirty_tiles->overwrite_tile(tile,mutable_tiles[tile*2]);
   }
  }
  if constexpr(KIND==9) {
   for(int i=0;i<MUTATIONS;++i) {
    int index=i+2,target=2+(i+frame)%30;
    palette->set_color(index,colors[target]);
   }
  }
  for(int i=0;i<OBJECTS;++i) {
   int cols=SPRITE_SIZE==8 ? 12:8, spacing=SPRITE_SIZE==8 ? 18:28;
   int x=4+(i%cols)*spacing+(frame%8),y=8+(i/cols)*18;
   sprites[i].set_position(x+SPRITE_SIZE/2-120,y+SPRITE_SIZE/2-80);
   if constexpr(KIND==2)sprites[i].set_tiles(animation_tiles[(frame/4)%4]);
  }
  if constexpr(KIND==14 || KIND==15) {
   for(int i=0;i<ENTITIES;++i)checksum=(checksum+map_cells[0][((i+frame)%32)*MAP_WIDTH+(i*7)%32])%16001;
   text_sprites.clear();text->generate_top_left(8,8,bn::to_string<16>(frame),text_sprites);
   for(auto& glyph:text_sprites)glyph.set_z_order(-1);
  }
  if constexpr(KIND==13) {
   bn::palette_bitmap_bg_painter painter(*bitmap);
   painter.fill(1);
   for(int i=0;i<ENTITIES;++i) {
    int x=(i*13+frame)%220,y=(i*17)%140;
    painter.rectangle(x,y,x+9,y+9,2+i%30);
   }
   painter.flip_page_later();
  }
  bn::core::update();
  unsigned cycles=bench_finish();
  if(cycles>max_cycles)max_cycles=cycles;
  missed+=bn::core::last_missed_frames();
  publish(tick,cycles);
 }
}
