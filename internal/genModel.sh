#!/usr/bin/env bash

# 使用方法：
# sh genModel.sh stabackend asset_detail


#生成的表名
tables=$2
#包名
modelPkgName=model
#表生成的genmodel目录
outPath="./model"
# 数据库配置
#host="172.30.14.123"
#port=3306
#dbname=$1
#username=dingo
#passwd=6XY3i+*hW4^5K7a2d@

host="172.30.14.123"
port=3307
dbname=$1
username=root
passwd=123123


echo "开始创建库：$dbname 的表：$2"
gentool -dsn "${username}:${passwd}@tcp(${host}:${port})/${dbname}?charset=utf8mb4&parseTime=True&loc=Local" -tables "${tables}" -onlyModel -modelPkgName="${modelPkgName}" -outPath="${outPath}"

