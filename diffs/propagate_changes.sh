#!/usr/bin/env bash

if [ -z "$1" ];
then
  echo "You must provide a changed main.go file"
  exit 1
fi

TARGET_FILE=$(realpath "$1")
LAST_FILE=''
for FILE in ../steps/**/main.go; do
  if [ "$(realpath "$LAST_FILE" 2> /dev/null)" == "$TARGET_FILE" ];
  then
    START_PROPAGATE=1
  fi

  if [ -n "$START_PROPAGATE" ];
  then
    DIFF_FILE="./$(basename "$(dirname "$FILE")").diff"
    patch -o "$(realpath $FILE)" $LAST_FILE $DIFF_FILE
  fi

  LAST_FILE=$FILE
done

if [ -z "$START_PROPAGATE" ];
then
  echo "You must provide a changed main.go file"
  exit 1
fi
