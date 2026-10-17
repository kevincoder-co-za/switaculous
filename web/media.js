(() => {
  'use strict';

  // Photos are scaled down and re-encoded as JPEG before they are sent, so a
  // 12-megapixel phone photo doesn't turn into a 10 MB value in the app (and in
  // the SwiftData store when the app saves it).
  const MAX_PHOTO_SIDE = 2048;
  const JPEG_QUALITY = 0.88;

  function element(tag, className, text) {
    const el = document.createElement(tag);
    if (className) el.className = className;
    if (text) el.textContent = text;
    return el;
  }

  function readFileAsDataURL(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(file);
    });
  }

  function loadImage(url) {
    return new Promise((resolve, reject) => {
      const img = new Image();
      img.onload = () => resolve(img);
      img.onerror = reject;
      img.src = url;
    });
  }

  function canvasToJPEG(source, width, height) {
    const scale = Math.min(1, MAX_PHOTO_SIDE / Math.max(width, height));
    const canvas = element('canvas');
    canvas.width = Math.round(width * scale);
    canvas.height = Math.round(height * scale);
    canvas.getContext('2d').drawImage(source, 0, 0, canvas.width, canvas.height);
    return canvas.toDataURL('image/jpeg', JPEG_QUALITY);
  }

  async function photoFromFile(file) {
    const isConvertibleImage = file.type.startsWith('image/') && file.type !== 'image/gif';
    if (isConvertibleImage) {
      const objectURL = URL.createObjectURL(file);
      try {
        const img = await loadImage(objectURL);
        return { name: file.name, type: 'image/jpeg', data: canvasToJPEG(img, img.naturalWidth, img.naturalHeight) };
      } catch (err) {
        // Formats the browser can't decode (for example HEIC outside Safari) are sent as they are.
      } finally {
        URL.revokeObjectURL(objectURL);
      }
    }
    return { name: file.name, type: file.type, data: await readFileAsDataURL(file) };
  }

  function chooseFiles({ multiple, accept }) {
    return new Promise((resolve) => {
      const input = element('input');
      input.type = 'file';
      input.accept = accept || 'image/*';
      input.multiple = !!multiple;
      input.style.display = 'none';
      document.body.appendChild(input);
      const finish = (files) => { input.remove(); resolve(files); };
      input.addEventListener('change', () => finish([...input.files]));
      input.addEventListener('cancel', () => finish([]));
      input.click();
    });
  }

  async function pickPhotos(options) {
    const files = await chooseFiles(options);
    const photos = [];
    for (const file of files) photos.push(await photoFromFile(file));
    return photos;
  }

  function stopWhenRemoved(host, stop) {
    const timer = setInterval(() => {
      if (!host.isConnected) { clearInterval(timer); stop(); }
    }, 500);
  }

  function mountCamera(host, { front, onPhoto, onCancel, onChooseFile }) {
    host.classList.add('camera-view');
    const video = element('video', 'camera-feed');
    video.autoplay = true;
    video.muted = true;
    video.playsInline = true;
    if (front) video.classList.add('mirrored');
    const still = element('img', 'camera-still');
    const message = element('div', 'camera-message');
    const controls = element('div', 'camera-controls');
    const cancelButton = element('button', 'camera-text-button', 'Cancel');
    const shutter = element('button', 'camera-shutter');
    shutter.title = 'Take photo';
    const spacer = element('div', 'camera-spacer');
    const retakeButton = element('button', 'camera-text-button', 'Retake');
    const useButton = element('button', 'camera-text-button', 'Use Photo');
    host.append(video, still, message, controls);

    let stream = null;
    let capturedPhoto = null;

    function showLiveControls() {
      controls.replaceChildren(cancelButton, shutter, spacer);
      still.style.display = 'none';
      video.style.display = '';
    }
    function showReviewControls() {
      controls.replaceChildren(retakeButton, spacer.cloneNode(), useButton);
      still.style.display = 'block';
      video.style.display = 'none';
    }
    function showNoCamera(reason) {
      video.style.display = 'none';
      message.replaceChildren(
        element('div', 'camera-message-title', 'No camera available'),
        element('div', 'camera-message-body', reason),
      );
      const chooseButton = element('button', 'camera-choose', 'Choose a Photo File…');
      chooseButton.addEventListener('click', onChooseFile);
      message.appendChild(chooseButton);
      controls.replaceChildren(cancelButton, spacer.cloneNode(), spacer.cloneNode());
    }

    cancelButton.addEventListener('click', onCancel);
    retakeButton.addEventListener('click', () => { capturedPhoto = null; showLiveControls(); });
    useButton.addEventListener('click', () => { if (capturedPhoto) onPhoto(capturedPhoto); });
    shutter.addEventListener('click', () => {
      if (!video.videoWidth) return;
      host.classList.add('flash');
      setTimeout(() => host.classList.remove('flash'), 150);
      const canvas = element('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      canvas.getContext('2d').drawImage(video, 0, 0);
      const data = canvasToJPEG(canvas, canvas.width, canvas.height);
      capturedPhoto = { name: 'Photo.jpg', type: 'image/jpeg', data };
      still.src = data;
      if (front) still.classList.add('mirrored');
      showReviewControls();
    });

    showLiveControls();
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
      showNoCamera('This browser does not allow camera access here.');
    } else {
      navigator.mediaDevices.getUserMedia({ video: { facingMode: front ? 'user' : 'environment', width: { ideal: 1920 } }, audio: false })
        .then((s) => {
          stream = s;
          if (!host.isConnected && host.dataset.stopped) { s.getTracks().forEach((t) => t.stop()); return; }
          video.srcObject = s;
        })
        .catch((err) => {
          const denied = err && (err.name === 'NotAllowedError' || err.name === 'SecurityError');
          showNoCamera(denied
            ? 'Camera access was blocked. Allow it in the browser’s address bar, or choose a photo file instead.'
            : 'No webcam was found on this computer. You can choose a photo file instead.');
        });
    }
    stopWhenRemoved(host, () => {
      host.dataset.stopped = '1';
      if (stream) stream.getTracks().forEach((t) => t.stop());
    });
  }

  function mountLibrary(host, { multiple, accept, onPick, onCancel }) {
    host.classList.add('library-view');
    const header = element('div', 'library-header');
    const cancelButton = element('button', 'library-cancel', 'Cancel');
    header.append(cancelButton, element('div', 'library-title', 'Photos'), element('div', 'library-spacer'));
    const body = element('div', 'library-body');
    const icon = element('div', 'library-icon');
    if (window.SFSymbol) icon.innerHTML = window.SFSymbol('photo.on.rectangle').svg;
    const chooseButton = element('button', 'library-choose', multiple ? 'Choose Photos…' : 'Choose a Photo…');
    const hint = element('div', 'library-hint', 'Pick images from this computer. On an iPhone this is the Photos library.');
    body.append(icon, chooseButton, hint);
    host.append(header, body);

    const choose = async () => {
      const photos = await pickPhotos({ multiple, accept });
      if (photos.length) onPick(photos);
    };
    cancelButton.addEventListener('click', onCancel);
    chooseButton.addEventListener('click', choose);
    // Browsers only open a file dialog in response to a click; the tap that
    // presented this sheet usually still counts, so try straight away.
    setTimeout(choose, 0);
  }

  window.SwitaculousMedia = { pickPhotos, mountCamera, mountLibrary };
})();
