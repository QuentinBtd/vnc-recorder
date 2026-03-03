#!/usr/bin/env python3
"""
VNC Recorder - Records a VNC session to video and uploads segments to S3
Uses vncdotool to capture frames from VNC
Compatible with IRSA for Kubernetes
"""

import os
import sys
import time
import subprocess
import signal
import logging
import threading
from pathlib import Path
from datetime import datetime
from typing import Optional
from io import BytesIO
import boto3
from botocore.exceptions import ClientError
from vncdotool import api

# Logging configuration
logging.basicConfig(
    level=logging.INFO,
    format="[%(asctime)s] %(levelname)s - %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger(__name__)

# Reduce log level of vncdotool (too verbose)
logging.getLogger("vncdotool").setLevel(logging.WARNING)
logging.getLogger("twisted").setLevel(logging.WARNING)


class VNCRecorder:
    """VNC session recorder with automatic S3 upload"""

    def __init__(self):
        """Initialize recorder with configuration from environment variables"""
        # Required configuration
        self.s3_bucket = os.environ.get("S3_BUCKET")
        if not self.s3_bucket:
            logger.error("ERROR: S3_BUCKET environment variable must be defined")
            sys.exit(1)

        # VNC configuration
        self.vnc_host = os.environ.get("VNC_HOST", "localhost")
        self.vnc_port = int(os.environ.get("VNC_PORT", "5900"))
        self.vnc_password = os.environ.get("VNC_PASSWORD", "")
        self.vnc_connect_max_retries = int(os.environ.get("VNC_CONNECT_MAX_RETRIES", "30"))
        self.vnc_connect_retry_delay = int(os.environ.get("VNC_CONNECT_RETRY_DELAY", "2"))

        # Video configuration
        self.segment_duration = int(os.environ.get("SEGMENT_DURATION", "300"))
        self.video_fps = int(os.environ.get("VIDEO_FPS", "25"))
        self.video_quality = int(os.environ.get("VIDEO_QUALITY", "23"))

        # S3 configuration
        self.s3_prefix = os.environ.get("S3_PREFIX", "")
        if self.s3_prefix and not self.s3_prefix.endswith("/"):
            self.s3_prefix = f"{self.s3_prefix}/"
        self.s3_segments_prefix = f"{self.s3_prefix}segments/"

        # Internal configuration
        self.output_dir = Path("/tmp/vnc-recordings")
        self.frames_dir = Path("/tmp/vnc-frames")
        self.output_dir.mkdir(exist_ok=True)
        self.frames_dir.mkdir(exist_ok=True)

        self.session_id = f"{datetime.now().strftime('%Y%m%d_%H%M%S')}_{os.getpid()}"

        # S3 client (automatic IRSA support)
        self.s3_client = boto3.client("s3")

        # VNC client
        self.vnc_client = None
        self.running = True
        self.current_segment = 0

        # Capture thread
        self.capture_thread = None

        # Configure signal handlers
        signal.signal(signal.SIGTERM, self._signal_handler)
        signal.signal(signal.SIGINT, self._signal_handler)

    def _signal_handler(self, signum, frame):
        """Handler for shutdown signals"""
        logger.info(f"Signal {signum} received, shutting down...")
        self.running = False

    def _create_vnc_client(self, server: str):
        password = self.vnc_password if self.vnc_password else None
        try:
            return api.connect(server, password=password, shared=True)
        except TypeError:
            return api.connect(server, password=password)

    def connect_vnc(self, max_retries: Optional[int] = None, retry_delay: Optional[int] = None) -> bool:
        """
        Connect to VNC server

        Args:
            max_retries: Maximum number of attempts
            retry_delay: Delay between attempts (seconds)

        Returns:
            True if connection is established, False otherwise
        """
        max_retries = max_retries or self.vnc_connect_max_retries
        retry_delay = retry_delay or self.vnc_connect_retry_delay
        logger.info(f"Connecting to VNC server {self.vnc_host}:{self.vnc_port}...")

        for attempt in range(1, max_retries + 1):
            try:
                # Build connection string
                server = f"{self.vnc_host}::{self.vnc_port}"

                # Connect to VNC server
                self.vnc_client = self._create_vnc_client(server)

                logger.info(f"✓ Connected to VNC server")
                return True

            except Exception as e:
                logger.debug(f"Attempt {attempt}/{max_retries} failed: {e}")

                if attempt < max_retries:
                    logger.info(f"Attempt {attempt}/{max_retries}...")
                    time.sleep(retry_delay)

        logger.error(f"✗ Unable to connect to VNC server after {max_retries} attempts")
        return False

    def reconnect_vnc(self) -> bool:
        logger.warning("VNC connection lost, attempting reconnection...")
        try:
            if self.vnc_client:
                self.vnc_client.disconnect()
        except Exception:
            pass
        self.vnc_client = None
        return self.connect_vnc()

    def capture_frames(self):
        """Continuously capture frames from VNC"""
        logger.info("Starting frame capture...")
        frame_count = 0
        segment_start_time = time.time()
        segment_start_frame = 0

        try:
            while self.running:
                frame_time = time.time()

                # Capture a frame
                try:
                    frame_path = self.frames_dir / f"frame_{frame_count:08d}.png"
                    self.vnc_client.captureScreen(str(frame_path))
                    frame_count += 1

                    # Check if we need to create a new segment
                    elapsed = time.time() - segment_start_time
                    if elapsed >= self.segment_duration:
                        # Create video segment
                        self.create_segment(segment_start_frame, frame_count - 1)

                        # New segment
                        segment_start_time = time.time()
                        segment_start_frame = frame_count
                        self.current_segment += 1

                except Exception as e:
                    logger.warning(f"Error capturing frame: {e}")
                    if not self.running:
                        break
                    if not self.reconnect_vnc():
                        logger.error("Unable to recover VNC connection")
                        self.running = False
                        break
                    continue

                # Wait to respect FPS
                elapsed_frame = time.time() - frame_time
                sleep_time = (1.0 / self.video_fps) - elapsed_frame
                if sleep_time > 0:
                    time.sleep(sleep_time)

        except Exception as e:
            logger.error(f"Error in frame capture: {e}", exc_info=True)
        finally:
            # Create last segment if there are remaining frames
            if frame_count > segment_start_frame:
                self.create_segment(segment_start_frame, frame_count - 1)

    def create_segment(self, start_frame: int, end_frame: int):
        """
        Create a video segment from captured frames

        Args:
            start_frame: First frame number
            end_frame: Last frame number
        """
        if end_frame <= start_frame:
            return

        segment_file = (
            self.output_dir
            / f"segment_{self.session_id}_{self.current_segment:03d}.mp4"
        )
        logger.info(
            f"Creating segment {self.current_segment} ({end_frame - start_frame + 1} frames)..."
        )

        try:
            # Use ffmpeg to create video from frames
            frame_pattern = str(self.frames_dir / f"frame_%08d.png")

            ffmpeg_cmd = [
                "ffmpeg",
                "-y",
                "-framerate",
                str(self.video_fps),
                "-start_number",
                str(start_frame),
                "-i",
                frame_pattern,
                "-frames:v",
                str(end_frame - start_frame + 1),
                "-c:v",
                "libx264",
                "-crf",
                str(self.video_quality),
                "-preset",
                "veryfast",
                "-pix_fmt",
                "yuv420p",
                str(segment_file),
            ]

            result = subprocess.run(
                ffmpeg_cmd, capture_output=True, text=True, timeout=60
            )

            if result.returncode == 0 and segment_file.exists():
                logger.info(
                    f"✓ Segment {self.current_segment} created: {segment_file.name}"
                )

                # Upload the segment
                self.upload_segment(segment_file)

                # Delete used frames
                self.cleanup_frames(start_frame, end_frame)
            else:
                logger.error(f"✗ Error creating segment: {result.stderr}")

        except Exception as e:
            logger.error(f"✗ Error creating segment: {e}")

    def cleanup_frames(self, start_frame: int, end_frame: int):
        """Delete frames that have been encoded into a segment"""
        for i in range(start_frame, end_frame + 1):
            frame_path = self.frames_dir / f"frame_{i:08d}.png"
            if frame_path.exists():
                try:
                    frame_path.unlink()
                except Exception as e:
                    logger.debug(f"Unable to delete {frame_path}: {e}")

    def upload_segment(self, file_path: Path) -> bool:
        """
        Upload a video segment to S3

        Args:
            file_path: Path to file to upload

        Returns:
            True if upload succeeded, False otherwise
        """
        s3_key = f"{self.s3_segments_prefix}{file_path.name}"
        s3_path = f"s3://{self.s3_bucket}/{s3_key}"

        try:
            logger.info(f"Upload: {file_path.name} → {s3_path}")

            self.s3_client.upload_file(str(file_path), self.s3_bucket, s3_key)

            logger.info(f"✓ Upload successful: {file_path.name}")

            # Delete local file after upload
            file_path.unlink()
            logger.debug(f"Local file deleted: {file_path.name}")

            return True

        except ClientError as e:
            logger.error(f"✗ Erreur upload S3: {e}")
            return False
        except Exception as e:
            logger.error(f"✗ Unexpected error during upload: {e}")
            return False

    def cleanup(self):
        """Clean up resources"""
        logger.info("Cleaning up...")

        # Close VNC connection
        if self.vnc_client:
            try:
                self.vnc_client.disconnect()
            except:
                pass

        # Delete remaining frames
        try:
            for frame_file in self.frames_dir.glob("frame_*.png"):
                frame_file.unlink()
            self.frames_dir.rmdir()
        except Exception as e:
            logger.debug(f"Error cleaning up frames: {e}")

        # Upload remaining segments
        remaining_segments = list(
            self.output_dir.glob(f"segment_{self.session_id}_*.mp4")
        )
        if remaining_segments:
            logger.info(f"Uploading {len(remaining_segments)} remaining segments...")
            for segment_file in remaining_segments:
                if segment_file.exists():
                    self.upload_segment(segment_file)

        logger.info("✓ Cleanup complete")

    def run(self):
        """Main entry point for the recorder"""
        logger.info("=" * 50)
        logger.info("VNC Recorder - Starting")
        logger.info("=" * 50)
        logger.info(f"Session ID: {self.session_id}")
        logger.info(f"VNC Server: {self.vnc_host}:{self.vnc_port}")
        logger.info(f"FPS: {self.video_fps}")
        logger.info(f"Segment duration: {self.segment_duration}s")
        logger.info(f"Video quality (CRF): {self.video_quality}")
        logger.info(f"S3 Destination: s3://{self.s3_bucket}/{self.s3_prefix}")
        logger.info("=" * 50)

        try:
            # Connect to VNC server
            if not self.connect_vnc():
                logger.error("Unable to connect to VNC server")
                return 1

            # Start frame capture
            self.capture_frames()

            return 0

        except Exception as e:
            logger.error(f"Fatal error: {e}", exc_info=True)
            return 1

        finally:
            # Cleanup
            self.cleanup()
            logger.info("=" * 50)
            logger.info("VNC Recorder - Finished")
            logger.info("=" * 50)


def main():
    """Main entry point"""
    recorder = VNCRecorder()
    sys.exit(recorder.run())


if __name__ == "__main__":
    main()
