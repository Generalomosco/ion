#!/bin/bash

APP_DIR=$(cd `dirname $0`/../; pwd)
cd $APP_DIR
mkdir -p $APP_DIR/logs

help()
{
    echo ""
    echo "start script"
    echo "Usage: ./allRestart.sh [-h]"
    echo ""
}

while getopts "h" arg
do
    case $arg in
        h)
            help;
            exit 0
            ;;
        ?)
            echo "No argument needed. Ignore them all!"
            ;;
    esac
done

# run command
echo "------------etcd--------------"
$APP_DIR/scripts/etcdStop.sh

echo "------------redis--------------"
$APP_DIR/scripts/redisStop.sh

echo "-----------rabbitmq---------------"
$APP_DIR/scripts/mqStop.sh

echo "------------islb--------------"
$APP_DIR/scripts/islbStop.sh

echo "------------ion--------------"
$APP_DIR/scripts/ionStop.sh


echo "------------web--------------"
$APP_DIR/scripts/webStop.sh
echo "--------------------------"



