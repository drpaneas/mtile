cmake_minimum_required(VERSION 3.15)
project(mtile-player C)

# mGBA 0.10.3 predates CMake 4's removal of compatibility policy defaults.
# This opts its unchanged CMake project into the oldest policy version still
# accepted by current CMake without editing the vendored submodule.
set(CMAKE_POLICY_VERSION_MINIMUM 3.5)

set(CMAKE_C_STANDARD 11)
set(CMAKE_C_STANDARD_REQUIRED ON)

# The editor helper needs only the GBA core. mGBA's LIBMGBA_ONLY path disables
# Qt/SDL frontends, optional media codecs, Lua, and network services and builds
# a small static library suitable for a hidden child process.
set(LIBMGBA_ONLY ON)
set(M_CORE_GBA ON)
set(M_CORE_GB OFF)
set(BINARY_NAME mgba-core)
add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../../third_party/mgba ${CMAKE_CURRENT_BINARY_DIR}/mgba EXCLUDE_FROM_ALL)

add_executable(mtile-player main.c)
target_link_libraries(mtile-player PRIVATE mgba-core)

if(MSVC)
  target_compile_options(mtile-player PRIVATE /W4)
else()
  target_compile_options(mtile-player PRIVATE -Wall -Wextra -Wpedantic)
endif()
