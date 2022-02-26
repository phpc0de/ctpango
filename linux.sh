#!/bin/sh

# how to use
# for macOS & linux, run this command in shell
# ./build.sh v0.1.0

name="ctpango"
version=$1

if [ "$1" = "" ]; then
  version=v1.0.0
fi

output="out"

default_golang() {
  export GOROOT=/usr/local/go
  go=$GOROOT/bin/go
}

Build() {
  default_golang
  goarm=$4
  if [ "$4" = "" ]; then
    goarm=7
  fi

  echo "Building $1..."
  export GOOS=$2 GOARCH=$3 GO386=sse2 CGO_ENABLED=0 GOARM=$4
  $go build -ldflags "-X main.Version=$version -s -w" -o "$output/$1/$name"
  RicePack $1 $name

  Pack $1 $2
}



# zip 打包
Pack() {
  if [ $2 != "windows" ]; then
      chmod +x "$output/$1/$name"
  fi

  cp README.md "$output/$1"

  cd $output
  zip -q -r "$1.zip" "$1"

  # 删除
  rm -rf "$1"

  cd ..
}

# rice 打包静态资源
RicePack() {
  return # 已取消web功能
}


# Linux
Build $name-$version"-linux-386" linux 386
Build $name-$version"-linux-amd64" linux amd64
Build $name-$version"-linux-armv5" linux arm 5
Build $name-$version"-linux-armv7" linux arm 7
Build $name-$version"-linux-arm64" linux arm64
GOMIPS=softfloat Build $name-$version"-linux-mips" linux mips
Build $name-$version"-linux-mips64" linux mips64
GOMIPS=softfloat Build $name-$version"-linux-mipsle" linux mipsle
Build $name-$version"-linux-mips64le" linux mips64le


# Others
Build $name-$version"-freebsd-386" freebsd 386
Build $name-$version"-freebsd-amd64" freebsd amd64
