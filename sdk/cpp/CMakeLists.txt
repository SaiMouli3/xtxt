cmake_minimum_required(VERSION 3.15)
project(xtxt_cpp LANGUAGES C CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_C_STANDARD 99)

# The C++ SDK is a wrapper, not a second parser: it compiles the C library and
# exposes it through RAII types, so the two cannot disagree about the format.
add_library(xtxt STATIC ../c/xtxt.c)
target_include_directories(xtxt PUBLIC ../c ${CMAKE_CURRENT_SOURCE_DIR})

if(MSVC)
  target_compile_options(xtxt PRIVATE /W4)
else()
  target_compile_options(xtxt PRIVATE -Wall -Wextra -Werror)
endif()

enable_testing()
add_executable(xtxt_tests tests/tests.cpp)
target_link_libraries(xtxt_tests PRIVATE xtxt)
if(NOT MSVC)
  target_compile_options(xtxt_tests PRIVATE -Wall -Wextra -Werror)
endif()
add_test(NAME xtxt_cpp COMMAND xtxt_tests "${CMAKE_CURRENT_SOURCE_DIR}/../../conformance/cases")
