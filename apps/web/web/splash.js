// Fades out the splash once Flutter draws its first frame.
window.addEventListener('flutter-first-frame', function () {
  var splash = document.getElementById('splash');
  if (!splash) return;
  splash.classList.add('gone');
  setTimeout(function () { splash.remove(); }, 300);
});
