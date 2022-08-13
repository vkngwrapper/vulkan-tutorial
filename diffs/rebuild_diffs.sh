#!/usr/bin/env bash

LAST_FILE=''
for FILE in ../steps/**/main.go; do
  if [ -n "$LAST_FILE" ]
  then
    DIFF_FILE="$(basename "$(dirname "$FILE")").diff"
    echo "Writing $DIFF_FILE"
    git diff --no-index "$LAST_FILE" "$FILE"  > $DIFF_FILE
  fi

  LAST_FILE=$FILE
done
