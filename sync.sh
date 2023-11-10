# linux 需要安装 gcc libpcap-dev
rsync -azvp --exclude .git ./ roots@192.168.1.33:l7dump/
echo '
export GOPROXY=https://goproxy.io,direct
cd l7dump
sudo CGO_ENABLED=1 go build -o /mnt/nfs/utils/l7dump .
' | ssh roots@192.168.1.33 'bash'
