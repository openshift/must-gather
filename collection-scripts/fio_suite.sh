#!/bin/bash

# Copied from peterducai/etcd_tools forked from openshift/etcd-tools

STAMP=$(date +%Y-%m-%d_%H-%M-%S)
ORIG_PATH=$(pwd)
OUTPUT_PATH="$ORIG_PATH/FIO-SUMMARY_$STAMP"
FSYNC_HIGH=15000
FSYNC_THRESHOLD=10000
FSYNC_EDGE=8000

echo -e "-------------------------------------------------------"
echo -e "FIO SUITE version 0.2.0"
echo -e "-------------------------------------------------------"
echo -e " "
echo -e "WARNING: this test can run for several minutes without any progress! Please wait until it finish!"
echo -e "START: $STAMP"
echo -e "All output can be found in  $OUTPUT_PATH"
echo -e "-------------------------------------------------------"
echo -e ""
echo -e ""

mkdir -p $OUTPUT_PATH
mkdir -p /test


# if [ -z "$(rpm -qa | grep fio)" ]
# then
#       echo "sudo dnf install fio -y"
# else
#       echo "fio is installed.. OK"
# fi


# echo -e " "
echo -e ""
echo -e "- [ SEQUENTIAL IOPS TEST ]-------------------------------------------------------"
echo -e ""

echo -e "  [ ETCD-like FSYNC WRITE with fsync engine]"
echo -e ""
echo -e "  the 99.0th and 99.9th percentile of this metric should be less than 10ms (10k)"
mkdir -p test-data
/usr/bin/fio --rw=write --ioengine=sync --fdatasync=1 --directory=test-data --size=22m --bs=8000 --name=cleanfsynctest > $OUTPUT_PATH/cleanfsynctest.log
echo -e ""
cat $OUTPUT_PATH/cleanfsynctest.log
echo -e ""
cat $OUTPUT_PATH/cleanfsynctest.log |grep "99.90th"|tail -1 > $OUTPUT_PATH/fsyncline
echo -e ""
echo -e "  IMPORTANT fsync percentiles:   $(cat $OUTPUT_PATH/fsyncline)"
sleep 2

cat $OUTPUT_PATH/fsyncline|cut -d ' ' -f8 |cut -d ']' -f 1 > $OUTPUT_PATH/fsync99
sleep 5
cat $OUTPUT_PATH/fsyncline|cut -d ' ' -f12 |cut -d ']' -f 1 > $OUTPUT_PATH/fsync999
sleep 5
FSYNC99=$(cat $OUTPUT_PATH/fsync99)
sleep 5
FSYNC999=$(cat $OUTPUT_PATH/fsync999)
sleep 5
IOPS=$(cat $OUTPUT_PATH/cleanfsynctest.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
if [[ "$IOPS" == *"k" ]]; then
  IOPS=$(echo $IOPS|rev|cut -c2-|rev)
  xIO=${IOPS%%.*}
  IOPS=$(( $xIO * 1000 ))
  #IOPS=$(($((${IOPS%%.*}))*1000))
fi
sleep 5
echo -e "-------------------------------------------------------"
echo -e "SEQUENTIAL IOPS: $IOPS"
if (( "$IOPS" < 300 )); then
    echo -e "  BAD.. IOPS is too low to run stable cluster.  $IOPS"
fi
if (( "$FSYNC99" > 10000 )); then
    echo -e "  BAD.. 99.0th fsync is higher than 10ms (10k).  $FSYNC99"
else
    echo -e "  OK.. 99.0th fsync is less than 10ms (10k).  $FSYNC99"
fi
if (( "$FSYNC999" > 10000 )); then
    echo -e "  BAD.. 99.9th fsync is higher than 10ms (10k).  $FSYNC999"
else
    echo -e "  OK.. 99.9th fsync is less than 10ms (10k).  $FSYNC999"
fi
if (( "$FSYNC999" > 8500 && "$FSYNC999" < 10000)); then
    echo -e "  WARNING.. 99.9th fsync is $FSYNC999 which is close to threshold 10ms (10k). Extra IO could make this value much worse."
fi
echo -e "-------------------------------------------------------"
echo -e ""


echo -e "  [ libaio engine SINGLE JOB, 70% read, 30% write]"
echo -e ""
echo -e "  This test is only for reference IOPS as it doesn't fully represent sequential IOPS of fsync."

/usr/bin/fio --name=seqread1g --filename=fiotest --runtime=120 --ioengine=libaio --direct=1 --ramp_time=10 --readwrite=rw --rwmixread=70 --rwmixwrite=30 --iodepth=1 --bs=4k --size=1G --percentage_random=0 > $OUTPUT_PATH/r70_w30_1G_d4.log
s7030big=$(cat $OUTPUT_PATH/r70_w30_1G_d4.log |grep IOPS|tail -2)
FSYNC=$(cat $OUTPUT_PATH/r70_w30_1G_d4.log |grep "99.00th"|tail -1|cut -c17-|grep -oE "([0-9]+)]" -m1|cut -d ']' -f 1|head -1)
wIOPS=$(cat $OUTPUT_PATH/r70_w30_1G_d4.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
rIOPS=$(cat $OUTPUT_PATH/r70_w30_1G_d4.log |grep IOPS|head -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
if [[ "$rIOPS" == *"k" ]]; then
  IOPS=$(echo $rIOPS|rev|cut -c2-|rev)
  xIO=${rIOPS%%.*}
  rIOPS=$(( $xIO * 1000 ))
fi
if [[ "$wIOPS" == *"k" ]]; then
  IOPS=$(echo $wIOPS|rev|cut -c2-|rev)
  xIO=${wIOPS%%.*}
  wIOPS=$(( $xIO * 1000 ))
fi

echo -e "1GB file transfer:"
echo -e "$s7030big"
echo -e ""
echo -e "SEQUENTIAL WRITE IOPS: $wIOPS"
echo -e "SEQUENTIAL READ IOPS: $rIOPS"
echo -e "--------------------------"
echo -e ""
/usr/bin/fio --name=seqread1mb --filename=fiotest --runtime=120 --ioengine=libaio --direct=1 --ramp_time=10  --readwrite=rw --rwmixread=70 --rwmixwrite=30 --iodepth=1 --bs=4k --size=200M > $OUTPUT_PATH/r70_w30_200M_d4.log
s7030small=$(cat $OUTPUT_PATH/r70_w30_200M_d4.log |grep IOPS|tail -2)
FSYNC=$(cat $OUTPUT_PATH/r70_w30_200M_d4.log |grep "99.00th"|tail -1|cut -c17-|grep -oE "([0-9]+)]" -m1|cut -d ']' -f 1|head -1)
wIOPS=$(cat $OUTPUT_PATH/r70_w30_200M_d4.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
rIOPS=$(cat $OUTPUT_PATH/r70_w30_200M_d4.log |grep IOPS|head -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
if [[ "$rIOPS" == *"k" ]]; then
  IOPS=$(echo $rIOPS|rev|cut -c2-|rev)
  xIO=${rIOPS%%.*}
  rIOPS=$(( $xIO * 1000 ))
fi
if [[ "$wIOPS" == *"k" ]]; then
  IOPS=$(echo $wIOPS|rev|cut -c2-|rev)
  xIO=${wIOPS%%.*}
  wIOPS=$(( $xIO * 1000 ))
fi

echo -e "200MB file transfer:"
echo -e "$s7030small"
echo -e ""
echo -e "SEQUENTIAL WRITE IOPS: $wIOPS"
echo -e "SEQUENTIAL READ IOPS: $rIOPS"
echo -e "--------------------------"


echo -e " "
echo -e "-- [ libaio engine SINGLE JOB, 30% read, 70% write] --"
echo -e " "

/usr/bin/fio --name=seqwrite1G --filename=fiotest --runtime=120 --bs=2k --ioengine=libaio --direct=1 --ramp_time=10 --readwrite=rw --rwmixread=30 --rwmixwrite=70 --iodepth=1 --bs=4k --size=200M  > $OUTPUT_PATH/r30_w70_200M_d1.log
so7030big=$(cat $OUTPUT_PATH/r30_w70_200M_d1.log |grep IOPS|tail -2)
FSYNC=$(cat $OUTPUT_PATH/r30_w70_200M_d1.log |grep "99.00th"|tail -1|cut -c17-|grep -oE "([0-9]+)]" -m1|cut -d ']' -f 1|head -1)
wIOPS=$(cat $OUTPUT_PATH/r30_w70_200M_d1.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
rIOPS=$(cat $OUTPUT_PATH/r30_w70_200M_d1.log |grep IOPS|head -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
if [[ "$rIOPS" == *"k" ]]; then
  IOPS=$(echo $rIOPS|rev|cut -c2-|rev)
  xIO=${rIOPS%%.*}
  rIOPS=$(( $xIO * 1000 ))
fi
if [[ "$wIOPS" == *"k" ]]; then
  IOPS=$(echo $wIOPS|rev|cut -c2-|rev)
  xIO=${wIOPS%%.*}
  wIOPS=$(( $xIO * 1000 ))
fi


echo -e "200MB file transfer:"
echo -e "$so7030big"
echo -e ""
echo -e "SEQUENTIAL WRITE IOPS: $wIOPS"
echo -e "SEQUENTIAL READ IOPS: $rIOPS"
echo -e "--------------------------"

# rm read*

echo -e " "
/usr/bin/fio --name=seqwrite1mb --filename=fiotest --runtime=120 --bs=2k --ioengine=libaio --direct=1 --ramp_time=10 --readwrite=rw --rwmixread=30 --rwmixwrite=70 --iodepth=1 --bs=4k --size=1G > $OUTPUT_PATH/r30_w70_1G_d1.log
so7030small=$(cat $OUTPUT_PATH/r30_w70_1G_d1.log |grep IOPS|tail -2)
FSYNC=$(cat $OUTPUT_PATH/r30_w70_1G_d1.log |grep "99.00th"|tail -1|cut -c17-|grep -oE "([0-9]+)]" -m1|cut -d ']' -f 1|head -1)
wIOPS=$(cat $OUTPUT_PATH/r30_w70_1G_d1.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
rIOPS=$(cat $OUTPUT_PATH/r30_w70_1G_d1.log |grep IOPS|head -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev|cut -c6-)
if [[ "$rIOPS" == *"k" ]]; then
  IOPS=$(echo $rIOPS|rev|cut -c2-|rev)
  xIO=${rIOPS%%.*}
  rIOPS=$(( $xIO * 1000 ))
fi
if [[ "$wIOPS" == *"k" ]]; then
  IOPS=$(echo $wIOPS|rev|cut -c2-|rev)
  xIO=${wIOPS%%.*}
  wIOPS=$(( $xIO * 1000 ))
fi


echo -e "1GB file transfer:"
echo -e "$so7030small"
echo -e ""
echo -e "SEQUENTIAL WRITE IOPS: $wIOPS"
echo -e "SEQUENTIAL READ IOPS: $rIOPS"
echo -e "--------------------------"


echo -e ""
echo -e ""
echo -e "[ RANDOM IOPS TEST ]-------------------------------------------------------"
echo -e ""
echo -e "[ RANDOM IOPS TEST ] - REQUEST OVERHEAD AND SEEK TIMES] ---"
echo -e ""
echo -e "This job is a latency-sensitive workload that stresses per-request overhead and seek times. Random reads."
echo -e ""

fio --name=seek1g --filename=fiotest --runtime=120 --ioengine=libaio --direct=1 --ramp_time=10 --iodepth=4 --readwrite=randread --blocksize=4k --size=1G > $OUTPUT_PATH/rand_1G_d1.log
#cat rand_1G_d1.log 
echo -e ""
overhead_big=$(cat $OUTPUT_PATH/rand_1G_d1.log |grep IOPS|tail -1)
FSYNC=$(cat $OUTPUT_PATH/rand_1G_d1.log |grep "99.00th"|tail -1|cut -c17-|grep -oE "([0-9]+)]" -m1|cut -d ']' -f 1|head -1)
IOPS=$(cat $OUTPUT_PATH/rand_1G_d1.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev)
if [[ "$IOPS" == *"k" ]]; then
  IOPS=$(echo $IOPS|rev|cut -c2-|rev)
  xIO=${IOPS%%.*}
  IOPS=$(( $xIO * 1000 ))
  #IOPS=$(($((${IOPS%%.*}))*1000))
fi

echo -e "1GB file transfer:"
echo -e "$overhead_big"
echo -e ""
echo -e "RANDOM IOPS: $IOPS"
echo -e "--------------------------"

echo -e ""
/usr/bin/fio --name=seek1mb --filename=fiotest --runtime=120 --ioengine=libaio --direct=1 --ramp_time=10 --iodepth=4  --readwrite=randread --blocksize=4k --size=200M > $OUTPUT_PATH/rand_200M_d1.log
overhead_small=$(cat $OUTPUT_PATH/rand_200M_d1.log |grep IOPS|tail -1)
FSYNC=$(cat $OUTPUT_PATH/rand_200M_d1.log |grep "99.00th"|tail -1|cut -c17-|grep -oE "([0-9]+)]" -m1|cut -d ']' -f 1|head -1)
IOPS=$(cat $OUTPUT_PATH/rand_200M_d1.log |grep IOPS|tail -1| cut -d ' ' -f2-|cut -d ' ' -f3|rev|cut -c2-|rev)
if [[ "$IOPS" == *"k" ]]; then
  IOPS=$(echo $IOPS|rev|cut -c2-|rev)
  xIO=${IOPS%%.*}
  IOPS=$(( $xIO * 1000 ))
fi

echo -e "200MB file transfer:"
echo -e "$overhead_small"
echo -e ""
echo -e "RANDOM IOPS: $IOPS"
echo -e "--------------------------"

rm fiotest
rm -rf test-data

echo -e " "
echo -e "- END -----------------------------------------"