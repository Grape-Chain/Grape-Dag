#!/bin/bash
#set -e
PARENT_PID=$$
SCRIPT_NAME=$(basename $0)
BASE_DIR=$(dirname "$(realpath "$0")")
LUNAPEER_PID=0
echo "*** *** ${SCRIPT_NAME} running by user=${USER} *** ***"

DEBUG=1
#Print debug message 
#$1 msg
debug() {
  local msg=$1
  local debug_msg="debug: ###### $msg ######"

  [ ${DEBUG} -eq 1 ] && echo "${debug_msg}"
}

#Print error message and exit with status 1
panic() {
  echo "Error: $*"
  exit 1
}

#Wait while te java server is starting
#$1 - server name
#$2 - timeout (default=15s)
#$3 - exit on fail if $3=="exit", otherwise returns status
#     default value="status"
wait_server_to_start() {
local server="$1"
local kwait=$2
local on_fail=$3
local count=0

  [ -z $kwait ] && kwait=15
  [ -z $on_fail ] && on_fail="status"

  while ! pgrep -U ${USER} -f ${server} &> /dev/null; do
    if [ "$on_fail" == "exit" ]; then       
      debug "Waiting for ${server} to start ..."
    fi
    sleep 1
    count=$((count + 1))
    if [ $count -gt $kwait ]; then 
       break
    fi
  done

  if ! pgrep -U ${USER} -f ${server} &> /dev/null ; then    
    if [ "$on_fail" == "exit" ]; then       
      echo "Error: ${server} not started after ${kwait} seconds."
      # panic "${server} not started."      
    else
      echo 1
    fi
  else
    echo 0  
  fi
}

#Wait while the peer is exiting
#$1 - server name
#$2 - timeout (default=30s)
wait_server_to_exit() {
local server="$1"
local kwait=$2
local count=0

  [ -z $kwait ] && kwait=30

  #while [ $(ps ax|grep -v ${SCRIPT_NAME}|grep ${server}|grep -v grep|awk '{print $1}'|wc -l) -gt 0 ]; do
  while pgrep -U ${USER} -f ${server} &> /dev/null; do
    debug "Waiting for ${server} to exit ..."
    sleep 1
    count=$((count + 1))
    if [ $count -gt $kwait ]; then 
       break
    fi
  done

  kill_process ${server}

}

#Wait while the peer is setting up
#$1 - timeout (default=15s)
wait_lunapeer_setup() {
local kwait=$1
local count=0
local proc_pid=0
  [ -z $kwait ] && kwait=15
  
  while true; do
    if [ -f ${PID_FILE} ]; then
      LUNAPEER_PID=$(cat ${PID_FILE}) 
      proc_pid=$(pgrep -f ${PEER_APP})
      #debug "LUNAPEER_PID=[${LUNAPEER_PID}] PROC_PID=[${proc_pid}])"
      if [ "${LUNAPEER_PID}" == "${proc_pid}" ]; then
        break
      fi      
    fi
    sleep 1  
    count=$((count + 1))
    if [ $count -gt $kwait ]; then 
      break       
    fi
  done
  if [ $count -gt $kwait ]; then 
    echo 1
  else
    echo 0
  fi    
}

#Kill process by given name
#$1 - name of service to be killed
kill_process() {
  local service=$1

  if [ -z ${service} ]; then 
    panic "Service name not specified"
  fi
  while pgrep -f ${service} &> /dev/null; do    
    debug "Kill process: ${service}"
    pkill -U ${USER} -f ${service}
    sleep 2
  done  
}

#Run script in WATCHDOG mode
start_watchdog_loop(){
  while true; do                
    rc=$(wait_server_to_start ${PEER_APP} 120 "status")
    debug "wait peer rc=${rc}"
    if [ "$rc" == "0" ]; then 
      debug "waiting for setup"
      rc=$(wait_lunapeer_setup 30)
      debug "Is peer running?, rc=${rc}"
      if [ "$rc" == "0" ]; then 
        "${TXGEN_APP}" -mode watchdog -timeout $WATCHDOG_TIMEOUT -retries $WATCHDOG_RETRIES &> /dev/null
        rc=$?
        echo "=== Watchdog has stopped, exit code=$rc"
        if [ "$rc" == "0" ]; then
          echo "=== Watchdog: Peer has received SUB response"          
        fi        
        exit 0
      fi  
    fi  
  done  
}

#Start Grape Virual Machine
start_vm() {
  VM_DIST="${HOME}/vm"
  VM_VERSION=$(cat ${VM_DIST}/VERSION.txt)
  VM_SERVER="vm-engine-server.jar"
  VM_SERVER_PATH="${VM_DIST}/${VM_SERVER}"
  
  debug "Using JAVA_HOME=${JAVA_HOME}"
  if type -p java; then
    debug "found java executable in PATH"
    _java=java
  elif [[ -n "$JAVA_HOME" ]] && [[ -x "$JAVA_HOME/bin/java" ]];  then
    debug "found java executable in JAVA_HOME"     
    _java="$JAVA_HOME/bin/java"
  else
    panic "java executable not found"
  fi

  #JAVA_OPTS="${JAVA_OPTS} -Xmx512m"
  [ -z ${GVM_SERVER_OPTS} ] && GVM_SERVER_OPTS="--disable-tracer --disable-mdLogger"

  echo "Run Grape VM Server v.${VM_SERVER}"
  echo "Using JAVA_OPTS=${JAVA_OPTS}"
  echo "Using GVM_SERVER_OPTS=${GVM_SERVER_OPTS}"
  echo
  echo "CMD>${_java} ${JAVA_OPTS} -jar "${VM_SERVER_PATH}" ${GVM_SERVER_OPTS}"
  ${_java} ${JAVA_OPTS} -jar "${VM_SERVER_PATH}" ${GVM_SERVER_OPTS} &> /dev/null &

  wait_server_to_start ${VM_SERVER} 30 exit
}

#Entry point
WATCHDOG="NO"
REST_ARGS=( )
MODULE="all"
DIST="/usr/local/bin"
CFG="${HOME}/.grap3/grapepeer.yml"
while [ $# -gt 0 ]; do
  case $1 in
    --host)
      HOST="$2"
      shift # past argument
      shift # past value
      ;;
    -f)
      CFG_FILE="$2"
      shift # past argument
      shift # past value
      ;;
    -id)
      PEER_ID="$2"
      REST_ARGS=("${REST_ARGS[@]}" "$1" "$2")
      shift # past argument
      shift # past value
      ;;  
    --vm)
      VM=YES
      shift # past argument
      ;;
    --vm-only)
      VM_ONLY=YES
      shift # past argument
      ;;
    --watchdog) 
      WATCHDOG=YES # 
      shift # past argument
      ;;
    --watchdog-daemon) 
      WATCHDOG_DAEMON=YES # runs 'txgeen -mode watchdog' in the background 
      shift # past argument
      ;;
    --timeout)
      WATCHDOG_TIMEOUT=$2
      shift # past argument
      shift # past value
      ;;
    --retries)
      WATCHDOG_RETRIES=$2
      shift # past argument
      shift # past value
      ;;
    --dist)  
      DIST="$2"
      shift # past argument
      shift # past value
      ;;
    --print-hosts)
      NW=YES
      shift # past argument
      ;;
    *)
      REST_ARGS=("${REST_ARGS[@]}" "$1") # save rest arg
      shift # past argument
      ;;
  esac
done

if [[ "YES" = "${NW}" ]]; then
  cat /etc/hosts
fi

if [[ "YES" == "${VM_ONLY}" ]]; then
  start_vm
  exit 0
fi

[ -z "${PEER_ID}" ] && echo "Error: Missing the mandatory argument: 'peer_id', use -id param" && exit 1
PID_FILE="${PEER_ID}.pid"
#set defaults
[ -z "${WATCHDOG_TIMEOUT}" ] && WATCHDOG_TIMEOUT=30
[ -z "${WATCHDOG_RETRIES}" ] && WATCHDOG_RETRIES=10

set -- "${REST_ARGS[@]}" # restore parameters
PEER_APP="${DIST}/grap3peer"
TXGEN_APP="${DIST}/txgen"
SERVICE="grap3peer"

if [[ "YES" == "${WATCHDOG_DAEMON}" ]]; then
  echo "Run Grape Peer WATCHDOG daemon"
  start_watchdog_loop
else
  [ -n "${HOST}" ] && debug "Argument host=${HOST}"
  [ -z "${HOST}" ] && HOST=$(awk 'END{print $1}' /etc/hosts)
  [ -z "${HOST}" ] && HOST="0.0.0.0"

  echo "Use: VM=${VM}, NW=${NW}, HOST=${HOST}, new config file=${CFG_FILE}"
  [ -n "${CFG_FILE}" ] && cp ${CFG_FILE} ${CFG}
  #Change host ip in the config
  sed -ri "s/host: \"(.+)\".*/host: \"${HOST}\"/g" "${CFG}"

  #Start VM 
  if [ "YES" = "${VM}" ]; then
    start_vm
  fi

  echo "Kill previously started gap3peer"
  kill_process "${PEER_APP}"

  if [ "YES" == "${WATCHDOG}" ]; then
    echo "Run Grape Peer with WATCHDOG"
    while true; do            
      echo "Kill previously started watchdog daemon and txgen"
      kill_process watchdog-daemon
      kill_process "${TXGEN_APP}"

      echo "Try to start Grape Peer WATCHDOG"
      "${BASE_DIR}/${SCRIPT_NAME}" --watchdog-daemon -id ${PEER_ID} --timeout ${WATCHDOG_TIMEOUT} --retries ${WATCHDOG_RETRIES} --dist ${DIST} &
      
      echo "CMD> ${PEER_APP} $@"
      "${PEER_APP}" $@
      rc=$?  
      echo "Grape Peer exit code=$rc"      
    done
  else
    echo "Run Grape Peer"
    echo "CMD> ${PEER_APP} $@"
    "${PEER_APP}" $@
  fi
fi
